package relay

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/dto"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/relay/channel"
	"github.com/QuantumNous/new-api/relay/channel/task/taskcommon"
	relaycommon "github.com/QuantumNous/new-api/relay/common"
	relayconstant "github.com/QuantumNous/new-api/relay/constant"
	"github.com/QuantumNous/new-api/relay/helper"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

type TaskSubmitResult struct {
	UpstreamTaskID string
	TaskData       []byte
	Platform       constant.TaskPlatform
	Quota          int
	//PerCallPrice   types.PriceData
}

// ResolveOriginTask 处理基于已有任务的提交（remix / continuation）：
// 查找原始任务、从中提取模型名称、将渠道锁定到原始任务的渠道
// （通过 info.LockedChannel，重试时复用同一渠道并轮换 key），
// 以及提取 OtherRatios（时长、分辨率）。
// 该函数在控制器的重试循环之前调用一次，其结果通过 info 字段和上下文持久化。
func ResolveOriginTask(c *gin.Context, info *relaycommon.RelayInfo) *dto.TaskError {
	// 检测 remix action
	path := c.Request.URL.Path
	if strings.Contains(path, "/v1/videos/") && strings.HasSuffix(path, "/remix") {
		info.Action = constant.TaskActionRemix
	}

	// 提取 remix 任务的 video_id
	if info.Action == constant.TaskActionRemix {
		videoID := c.Param("video_id")
		if strings.TrimSpace(videoID) == "" {
			return service.TaskErrorWrapperLocal(fmt.Errorf("video_id is required"), "invalid_request", http.StatusBadRequest)
		}
		info.OriginTaskID = videoID
	}

	if info.OriginTaskID == "" {
		return nil
	}

	// 查找原始任务
	originTask, exist, err := model.GetByTaskId(info.UserId, info.OriginTaskID)
	if err != nil {
		return service.TaskErrorWrapper(err, "get_origin_task_failed", http.StatusInternalServerError)
	}
	if !exist {
		return service.TaskErrorWrapperLocal(errors.New("task_origin_not_exist"), "task_not_exist", http.StatusBadRequest)
	}

	// 从原始任务推导模型名称
	if info.OriginModelName == "" {
		if originTask.Properties.OriginModelName != "" {
			info.OriginModelName = originTask.Properties.OriginModelName
		} else if originTask.Properties.UpstreamModelName != "" {
			info.OriginModelName = originTask.Properties.UpstreamModelName
		} else {
			var taskData map[string]interface{}
			_ = common.Unmarshal(originTask.Data, &taskData)
			if m, ok := taskData["model"].(string); ok && m != "" {
				info.OriginModelName = m
			}
		}
	}

	// 锁定到原始任务的渠道（重试时复用同一渠道，轮换 key）
	ch, err := model.GetChannelById(originTask.ChannelId, true)
	if err != nil {
		return service.TaskErrorWrapperLocal(err, "channel_not_found", http.StatusBadRequest)
	}
	if ch.Status != common.ChannelStatusEnabled {
		return service.TaskErrorWrapperLocal(errors.New("the channel of the origin task is disabled"), "task_channel_disable", http.StatusBadRequest)
	}
	info.LockedChannel = ch

	if originTask.ChannelId != info.ChannelId {
		key, _, newAPIError := ch.GetNextEnabledKey()
		if newAPIError != nil {
			return service.TaskErrorWrapper(newAPIError, "channel_no_available_key", newAPIError.StatusCode)
		}
		common.SetContextKey(c, constant.ContextKeyChannelKey, key)
		common.SetContextKey(c, constant.ContextKeyChannelType, ch.Type)
		common.SetContextKey(c, constant.ContextKeyChannelBaseUrl, ch.GetBaseURL())
		common.SetContextKey(c, constant.ContextKeyChannelId, originTask.ChannelId)

		info.ChannelBaseUrl = ch.GetBaseURL()
		info.ChannelId = originTask.ChannelId
		info.ChannelType = ch.Type
		info.ApiKey = key
	}

	// 提取 remix 参数（时长、分辨率 → OtherRatios）
	if info.Action == constant.TaskActionRemix {
		if originTask.PrivateData.BillingContext != nil {
			// 新的 remix 逻辑：直接从原始任务的 BillingContext 中提取 OtherRatios（如果存在）
			for s, f := range originTask.PrivateData.BillingContext.OtherRatios {
				info.PriceData.AddOtherRatio(s, f)
			}
		} else {
			// 旧的 remix 逻辑：直接从 task data 解析 seconds 和 size（如果存在）
			var taskData map[string]interface{}
			_ = common.Unmarshal(originTask.Data, &taskData)
			secondsStr, _ := taskData["seconds"].(string)
			seconds, _ := strconv.Atoi(secondsStr)
			if seconds <= 0 {
				seconds = 4
			}
			sizeStr, _ := taskData["size"].(string)
			if info.PriceData.OtherRatios == nil {
				info.PriceData.OtherRatios = map[string]float64{}
			}
			info.PriceData.OtherRatios["seconds"] = float64(seconds)
			info.PriceData.OtherRatios["size"] = 1
			if sizeStr == "1792x1024" || sizeStr == "1024x1792" {
				info.PriceData.OtherRatios["size"] = 1.666667
			}
		}
	}

	return nil
}

// RelayTaskSubmit 完成 task 提交的全部流程（每次尝试调用一次）：
// 刷新渠道元数据 → 确定 platform/adaptor → 验证请求 →
// 估算计费(EstimateBilling) → 计算价格 → 预扣费（仅首次）→
// 构建/发送/解析上游请求 → 提交后计费调整(AdjustBillingOnSubmit)。
// 控制器负责 defer Refund 和成功后 Settle。
func RelayTaskSubmit(c *gin.Context, info *relaycommon.RelayInfo) (*TaskSubmitResult, *dto.TaskError) {
	info.InitChannelMeta(c)

	// 1. 确定 platform → 创建适配器 → 验证请求
	platform := constant.TaskPlatform(c.GetString("platform"))
	if platform == "" {
		platform = GetTaskPlatform(c)
	}
	adaptor := GetTaskAdaptor(platform)
	if adaptor == nil {
		return nil, service.TaskErrorWrapperLocal(fmt.Errorf("invalid api platform: %s", platform), "invalid_api_platform", http.StatusBadRequest)
	}
	adaptor.Init(info)
	if taskErr := adaptor.ValidateRequestAndSetAction(c, info); taskErr != nil {
		return nil, taskErr
	}

	// 2. 确定模型名称
	modelName := info.OriginModelName
	if modelName == "" {
		modelName = service.CoverTaskActionToModelName(platform, info.Action)
	}

	// 2.5 应用渠道的模型映射（与同步任务对齐）
	info.OriginModelName = modelName
	info.UpstreamModelName = modelName
	if err := helper.ModelMappedHelper(c, info, nil); err != nil {
		return nil, service.TaskErrorWrapperLocal(err, "model_mapping_failed", http.StatusBadRequest)
	}

	// 3. 预生成公开 task ID（仅首次）
	if info.PublicTaskID == "" {
		info.PublicTaskID = model.GenerateTaskID()
	}

	// 4. 价格计算：基础模型价格
	info.OriginModelName = modelName
	priceData, err := helper.ModelPriceHelperPerCall(c, info)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "model_price_error", http.StatusBadRequest)
	}
	info.PriceData = priceData

	// 5. 计费估算：让适配器根据用户请求提供 OtherRatios（时长、分辨率等）
	//    必须在 ModelPriceHelperPerCall 之后调用（它会重建 PriceData）。
	//    ResolveOriginTask 可能已在 remix 路径中预设了 OtherRatios，此处合并。
	if estimatedRatios := adaptor.EstimateBilling(c, info); len(estimatedRatios) > 0 {
		for k, v := range estimatedRatios {
			info.PriceData.AddOtherRatio(k, v)
		}
	}

	// 6. 将 OtherRatios 应用到基础额度
	if !common.StringsContains(constant.TaskPricePatches, modelName) {
		for _, ra := range info.PriceData.OtherRatios {
			if ra != 1.0 {
				info.PriceData.Quota = int(float64(info.PriceData.Quota) * ra)
			}
		}
	}

	// 7. 预扣费（仅首次 — 重试时 info.Billing 已存在，跳过）
	if info.Billing == nil && !info.PriceData.FreeModel {
		info.ForcePreConsume = true
		if apiErr := service.PreConsumeBilling(c, info.PriceData.Quota, info); apiErr != nil {
			return nil, service.TaskErrorFromAPIError(apiErr)
		}
	}

	// 8. 构建请求体
	requestBody, err := adaptor.BuildRequestBody(c, info)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "build_request_failed", http.StatusInternalServerError)
	}

	// 9. 发送请求
	resp, err := adaptor.DoRequest(c, info, requestBody)
	if err != nil {
		return nil, service.TaskErrorWrapper(err, "do_request_failed", http.StatusInternalServerError)
	}
	if resp != nil && resp.StatusCode != http.StatusOK {
		responseBody, _ := io.ReadAll(resp.Body)
		return nil, service.TaskErrorWrapper(fmt.Errorf("%s", string(responseBody)), "fail_to_fetch_task", resp.StatusCode)
	}

	// 10. 返回 OtherRatios 给下游（header 必须在 DoResponse 写 body 之前设置）
	otherRatios := info.PriceData.OtherRatios
	if otherRatios == nil {
		otherRatios = map[string]float64{}
	}
	ratiosJSON, _ := common.Marshal(otherRatios)
	c.Header("X-New-Api-Other-Ratios", string(ratiosJSON))

	// 11. 解析响应
	upstreamTaskID, taskData, taskErr := adaptor.DoResponse(c, resp, info)
	if taskErr != nil {
		return nil, taskErr
	}

	// 11. 提交后计费调整：让适配器根据上游实际返回调整 OtherRatios
	finalQuota := info.PriceData.Quota
	if adjustedRatios := adaptor.AdjustBillingOnSubmit(info, taskData); len(adjustedRatios) > 0 {
		// 基于调整后的 ratios 重新计算 quota
		finalQuota = recalcQuotaFromRatios(info, adjustedRatios)
		info.PriceData.OtherRatios = adjustedRatios
		info.PriceData.Quota = finalQuota
	}

	return &TaskSubmitResult{
		UpstreamTaskID: upstreamTaskID,
		TaskData:       taskData,
		Platform:       platform,
		Quota:          finalQuota,
	}, nil
}

// recalcQuotaFromRatios 根据 adjustedRatios 重新计算 quota。
// 公式: baseQuota × ∏(ratio) — 其中 baseQuota 是不含 OtherRatios 的基础额度。
func recalcQuotaFromRatios(info *relaycommon.RelayInfo, ratios map[string]float64) int {
	// 从 PriceData 获取不含 OtherRatios 的基础价格
	baseQuota := info.PriceData.Quota
	// 先除掉原有的 OtherRatios 恢复基础额度
	for _, ra := range info.PriceData.OtherRatios {
		if ra != 1.0 && ra > 0 {
			baseQuota = int(float64(baseQuota) / ra)
		}
	}
	// 应用新的 ratios
	result := float64(baseQuota)
	for _, ra := range ratios {
		if ra != 1.0 {
			result *= ra
		}
	}
	return int(result)
}

var fetchRespBuilders = map[int]func(c *gin.Context) (respBody []byte, taskResp *dto.TaskError){
	relayconstant.RelayModeSunoFetchByID:        sunoFetchByIDRespBodyBuilder,
	relayconstant.RelayModeSunoFetch:            sunoFetchRespBodyBuilder,
	relayconstant.RelayModeVideoFetchByID:       videoFetchByIDRespBodyBuilder,
	relayconstant.RelayModeImageTaskFetchByID:   imageTaskFetchByIDRespBodyBuilder,
}

func RelayTaskFetch(c *gin.Context, relayMode int) (taskResp *dto.TaskError) {
	respBuilder, ok := fetchRespBuilders[relayMode]
	if !ok {
		taskResp = service.TaskErrorWrapperLocal(errors.New("invalid_relay_mode"), "invalid_relay_mode", http.StatusBadRequest)
	}

	respBody, taskErr := respBuilder(c)
	if taskErr != nil {
		return taskErr
	}
	if len(respBody) == 0 {
		respBody = []byte("{\"code\":\"success\",\"data\":null}")
	}

	c.Writer.Header().Set("Content-Type", "application/json")
	_, err := io.Copy(c.Writer, bytes.NewBuffer(respBody))
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "copy_response_body_failed", http.StatusInternalServerError)
		return
	}
	return
}

func sunoFetchRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	userId := c.GetInt("id")
	var condition = struct {
		IDs    []any  `json:"ids"`
		Action string `json:"action"`
	}{}
	err := c.BindJSON(&condition)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "invalid_request", http.StatusBadRequest)
		return
	}
	var tasks []any
	if len(condition.IDs) > 0 {
		taskModels, err := model.GetByTaskIds(userId, condition.IDs)
		if err != nil {
			taskResp = service.TaskErrorWrapper(err, "get_tasks_failed", http.StatusInternalServerError)
			return
		}
		for _, task := range taskModels {
			tasks = append(tasks, TaskModel2Dto(task))
		}
	} else {
		tasks = make([]any, 0)
	}
	respBody, err = common.Marshal(dto.TaskResponse[[]any]{
		Code: "success",
		Data: tasks,
	})
	return
}

func sunoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("id")
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	respBody, err = common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: TaskModel2Dto(originTask),
	})
	return
}

func videoFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("task_id")
	if taskId == "" {
		taskId = c.GetString("task_id")
	}
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	isOpenAIVideoAPI := strings.HasPrefix(c.Request.RequestURI, "/v1/videos/")
	isDoubaoOfficialAPI := strings.Contains(c.Request.RequestURI, "/api/v3/contents/generations/tasks")

	// Gemini/Vertex 支持实时查询：用户 fetch 时直接从上游拉取最新状态
	if realtimeResp := tryRealtimeFetch(originTask, isOpenAIVideoAPI); len(realtimeResp) > 0 {
		respBody = realtimeResp
		return
	}

	// Doubao official API 格式: 返回 doubao responseTask 格式
	if isDoubaoOfficialAPI {
		// 尝试从上游实时拉取最新状态（当本地Data字段不完整时）
		if realtimeResp := tryDoubaoRealtimeFetch(originTask, c); len(realtimeResp) > 0 {
			respBody = realtimeResp
			return
		}

		// 直接返回存储的上游响应数据（doubao 官方格式）
		// task.Data 中的 status 已经是官方格式（succeeded/running/queued等），不需要转换
		var taskData map[string]interface{}
		if err := common.Unmarshal(originTask.Data, &taskData); err == nil {
			// 删除 upstream_task_id 字段，保持与 doubao 官方格式一致
			delete(taskData, "upstream_task_id")
			// 替换为我们的 public task ID（task.Data 中的 id 是上游 ID）
			taskData["id"] = originTask.TaskID

			// 如果 task.Data 中没有 status（刚创建时），使用本地状态并转换为官方格式
			if _, hasStatus := taskData["status"]; !hasStatus {
				taskData["status"] = originTask.Status.ToDoubaoStatus()
			}

			// 不需要转换 status：task.Data 存储的是上游原始响应，status 已经是官方格式
			respBody, _ = common.Marshal(taskData)
		} else {
			respBody = originTask.Data
		}
		return
	}

	// OpenAI Video API 格式: 走各 adaptor 的 ConvertToOpenAIVideo
	if isOpenAIVideoAPI {
		// 尝试从上游实时拉取最新状态（如果开启了强制查询）
		if realtimeResp := tryOpenAIVideoRealtimeFetch(originTask, c); len(realtimeResp) > 0 {
			respBody = realtimeResp
			return
		}

		adaptor := GetTaskAdaptor(originTask.Platform)
		if adaptor == nil {
			taskResp = service.TaskErrorWrapperLocal(fmt.Errorf("invalid channel id: %d", originTask.ChannelId), "invalid_channel_id", http.StatusBadRequest)
			return
		}
		if converter, ok := adaptor.(channel.OpenAIVideoConverter); ok {
			openAIVideoData, err := converter.ConvertToOpenAIVideo(originTask)
			if err != nil {
				taskResp = service.TaskErrorWrapper(err, "convert_to_openai_video_failed", http.StatusInternalServerError)
				return
			}
			respBody = openAIVideoData
			return
		}
		taskResp = service.TaskErrorWrapperLocal(fmt.Errorf("not_implemented:%s", originTask.Platform), "not_implemented", http.StatusNotImplemented)
		return
	}

	// 通用 TaskDto 格式
	respBody, err = common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: TaskModel2Dto(originTask),
	})
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	return
}

// tryDoubaoRealtimeFetch 尝试从上游实时拉取 Doubao 任务状态。
// 默认仅当 task.Data 字段不完整时触发实时查询。
// 可通过渠道配置 doubao_video_always_fetch_upstream=true 强制每次都查询上游。
// 查询成功后更新本地任务状态，并返回 Doubao 官方格式的响应体。
func tryDoubaoRealtimeFetch(task *model.Task, c *gin.Context) []byte {
	// 获取渠道配置
	channelModel, err := model.GetChannelById(task.ChannelId, true)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("[Doubao实时查询] 获取渠道失败: %v", err))
		return nil
	}

	settings := channelModel.GetSetting()
	alwaysFetch := false
	if channelModel.Other != "" {
		var otherSettings dto.ChannelOtherSettings
		if err := common.Unmarshal([]byte(channelModel.Other), &otherSettings); err == nil {
			alwaysFetch = otherSettings.DoubaoVideoAlwaysFetchUpstream
		}
	}

	// 检查 task.Data 是否完整
	var taskData map[string]interface{}
	if err := common.Unmarshal(task.Data, &taskData); err != nil {
		return nil
	}

	dataComplete := false
	if _, hasStatus := taskData["status"]; hasStatus {
		dataComplete = true
	}
	if _, hasModel := taskData["model"]; hasModel {
		dataComplete = true
	}
	if _, hasCreatedAt := taskData["created_at"]; hasCreatedAt {
		dataComplete = true
	}

	// 判断是否需要查询上游
	shouldFetch := false
	if alwaysFetch {
		// 强制实时查询模式
		shouldFetch = true
		logger.LogInfo(c, fmt.Sprintf("[Doubao实时查询] 强制实时查询模式，任务 %s 发起上游查询", task.TaskID))
	} else if !dataComplete {
		// Data 不完整，需要实时查询上游
		shouldFetch = true
		logger.LogInfo(c, fmt.Sprintf("[Doubao实时查询] 任务 %s 的 Data 字段不完整，发起上游查询", task.TaskID))
	}

	if !shouldFetch {
		// Data 完整且未开启强制查询，不需要实时查询
		return nil
	}

	baseURL := constant.ChannelBaseURLs[channelModel.Type]
	if channelModel.GetBaseURL() != "" {
		baseURL = channelModel.GetBaseURL()
	}
	proxy := settings.Proxy
	adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(channelModel.Type)))
	if adaptor == nil {
		logger.LogError(c, "[Doubao实时查询] 获取适配器失败")
		return nil
	}

	upstreamTaskID := task.GetUpstreamTaskID()
	logger.LogInfo(c, fmt.Sprintf("[Doubao实时查询] 查询上游任务: %s", upstreamTaskID))
	logger.LogInfo(c, fmt.Sprintf("[Doubao实时查询] baseURL: %s", baseURL))
	logger.LogInfo(c, fmt.Sprintf("[Doubao实时查询] 渠道类型: %d", channelModel.Type))

	resp, err := adaptor.FetchTask(baseURL, channelModel.Key, map[string]any{
		"task_id": upstreamTaskID,
		"action":  task.Action,
	}, proxy)
	if err != nil || resp == nil {
		logger.LogError(c, fmt.Sprintf("[Doubao实时查询] 查询上游失败: %v", err))
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("[Doubao实时查询] 读取响应失败: %v", err))
		return nil
	}

	logger.LogInfo(c, fmt.Sprintf("[Doubao实时查询] 上游响应状态码: %d", resp.StatusCode))
	logger.LogInfo(c, fmt.Sprintf("[Doubao实时查询] 上游响应体: %s", string(body)))

	logger.LogInfo(c, fmt.Sprintf("[Doubao实时查询] 上游响应: %s", string(body)))

	ti, err := adaptor.ParseTaskResult(body)
	if err != nil || ti == nil {
		logger.LogError(c, fmt.Sprintf("[Doubao实时查询] 解析响应失败: %v", err))
		return nil
	}

	snap := task.Snapshot()

	// 更新任务状态
	if ti.Status != "" {
		task.Status = model.TaskStatus(ti.Status)
	}
	if ti.Progress != "" {
		task.Progress = ti.Progress
	}
	if ti.Url != "" {
		task.PrivateData.ResultURL = ti.Url
	}

	// 保存上游完整的原始响应到 Data 字段（不修改，便于调试和分析）
	task.Data = body

	// 保存到数据库
	if !snap.Equal(task.Snapshot()) {
		_, err := task.UpdateWithStatus(snap.Status)
		if err != nil {
			logger.LogError(c, fmt.Sprintf("[Doubao实时查询] 更新任务失败: %v", err))
		} else {
			logger.LogInfo(c, fmt.Sprintf("[Doubao实时查询] 任务 %s 状态已更新: %s", task.TaskID, ti.Status))
		}
	}

	// 构建返回给下游的响应：删除内部字段，保持 id 为上游任务ID
	var responseData map[string]interface{}
	if err := common.Unmarshal(body, &responseData); err == nil {
		// 删除上游的内部字段
		delete(responseData, "upstream_task_id")
		// id 字段保持为上游任务ID（用户查询时使用的ID）
		responseData["id"] = task.TaskID

		if respBody, err := common.Marshal(responseData); err == nil {
			return respBody
		}
	}

	return body
}

// tryOpenAIVideoRealtimeFetch 尝试从上游实时拉取任务状态（OpenAI Video API 格式）。
// 默认情况下 OpenAI Video API 格式永不调用上游，总是读取数据库。
// 可通过渠道配置 openai_video_always_fetch_upstream=true 强制每次都查询上游。
// 查询成功后更新本地任务状态，并返回 OpenAI Video 格式的响应体。
func tryOpenAIVideoRealtimeFetch(task *model.Task, c *gin.Context) []byte {
	// 获取渠道配置
	channelModel, err := model.GetChannelById(task.ChannelId, true)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("[OpenAI Video实时查询] 获取渠道失败: %v", err))
		return nil
	}

	// 检查是否开启了强制查询
	alwaysFetch := false
	if channelModel.Other != "" {
		var otherSettings dto.ChannelOtherSettings
		if err := common.Unmarshal([]byte(channelModel.Other), &otherSettings); err == nil {
			alwaysFetch = otherSettings.OpenAIVideoAlwaysFetchUpstream
		}
	}

	if !alwaysFetch {
		// 未开启强制查询，使用默认行为（不查询上游）
		return nil
	}

	// 强制实时查询模式
	logger.LogInfo(c, fmt.Sprintf("[OpenAI Video实时查询] 强制实时查询模式，任务 %s 发起上游查询", task.TaskID))

	settings := channelModel.GetSetting()
	baseURL := constant.ChannelBaseURLs[channelModel.Type]
	if channelModel.GetBaseURL() != "" {
		baseURL = channelModel.GetBaseURL()
	}
	proxy := settings.Proxy
	adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(channelModel.Type)))
	if adaptor == nil {
		logger.LogError(c, "[OpenAI Video实时查询] 获取适配器失败")
		return nil
	}

	upstreamTaskID := task.GetUpstreamTaskID()
	logger.LogInfo(c, fmt.Sprintf("[OpenAI Video实时查询] 查询上游任务: %s", upstreamTaskID))
	logger.LogInfo(c, fmt.Sprintf("[OpenAI Video实时查询] baseURL: %s", baseURL))
	logger.LogInfo(c, fmt.Sprintf("[OpenAI Video实时查询] 渠道类型: %d", channelModel.Type))

	resp, err := adaptor.FetchTask(baseURL, channelModel.Key, map[string]any{
		"task_id": upstreamTaskID,
		"action":  task.Action,
	}, proxy)
	if err != nil || resp == nil {
		logger.LogError(c, fmt.Sprintf("[OpenAI Video实时查询] 查询上游失败: %v", err))
		return nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("[OpenAI Video实时查询] 读取响应失败: %v", err))
		return nil
	}

	logger.LogInfo(c, fmt.Sprintf("[OpenAI Video实时查询] 上游响应状态码: %d", resp.StatusCode))
	logger.LogInfo(c, fmt.Sprintf("[OpenAI Video实时查询] 上游响应体: %s", string(body)))

	if resp.StatusCode != http.StatusOK {
		logger.LogError(c, fmt.Sprintf("[OpenAI Video实时查询] 上游返回错误: %d, %s", resp.StatusCode, string(body)))
		return nil
	}

	// 解析上游响应（Doubao 格式）
	var upstreamResp map[string]interface{}
	if err := common.Unmarshal(body, &upstreamResp); err != nil {
		logger.LogError(c, fmt.Sprintf("[OpenAI Video实时查询] 解析响应失败: %v", err))
		return nil
	}

	// 更新本地任务状态
	snap := task.Snapshot()

	// 提取状态信息
	if status, ok := upstreamResp["status"].(string); ok {
		task.Status = model.DoubaoStatusToInternal(status)
	}
	if progress, ok := upstreamResp["progress"].(float64); ok {
		task.Progress = fmt.Sprintf("%.0f", progress)
	}
	if videoResults, ok := upstreamResp["video_result"].([]interface{}); ok && len(videoResults) > 0 {
		if firstResult, ok := videoResults[0].(map[string]interface{}); ok {
			if url, ok := firstResult["url"].(string); ok {
				task.PrivateData.ResultURL = url
			}
		}
	}

	// 保存上游完整的原始响应到 Data 字段
	task.Data = body

	// 保存到数据库
	if !snap.Equal(task.Snapshot()) {
		_, err := task.UpdateWithStatus(snap.Status)
		if err != nil {
			logger.LogError(c, fmt.Sprintf("[OpenAI Video实时查询] 更新任务失败: %v", err))
		} else {
			logger.LogInfo(c, fmt.Sprintf("[OpenAI Video实时查询] 任务 %s 状态已更新: %s", task.TaskID, task.Status))
		}
	}

	// 转换为 OpenAI Video 格式
	converter, ok := adaptor.(channel.OpenAIVideoConverter)
	if !ok {
		logger.LogError(c, "[OpenAI Video实时查询] 适配器不支持 OpenAI Video 格式转换")
		return nil
	}

	openAIVideoData, err := converter.ConvertToOpenAIVideo(task)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("[OpenAI Video实时查询] 转换为 OpenAI Video 格式失败: %v", err))
		return nil
	}

	logger.LogInfo(c, fmt.Sprintf("[OpenAI Video实时查询] 查询成功，返回 OpenAI Video 格式"))
	return openAIVideoData
}

// tryRealtimeFetch 尝试从上游实时拉取 Gemini/Vertex 任务状态。
// 仅当渠道类型为 Gemini 或 Vertex 时触发；其他渠道或出错时返回 nil。
// 当非 OpenAI Video API 时，还会构建自定义格式的响应体。
func tryRealtimeFetch(task *model.Task, isOpenAIVideoAPI bool) []byte {
	channelModel, err := model.GetChannelById(task.ChannelId, true)
	if err != nil {
		return nil
	}
	if channelModel.Type != constant.ChannelTypeVertexAi && channelModel.Type != constant.ChannelTypeGemini {
		return nil
	}

	baseURL := constant.ChannelBaseURLs[channelModel.Type]
	if channelModel.GetBaseURL() != "" {
		baseURL = channelModel.GetBaseURL()
	}
	proxy := channelModel.GetSetting().Proxy
	adaptor := GetTaskAdaptor(constant.TaskPlatform(strconv.Itoa(channelModel.Type)))
	if adaptor == nil {
		return nil
	}

	resp, err := adaptor.FetchTask(baseURL, channelModel.Key, map[string]any{
		"task_id": task.GetUpstreamTaskID(),
		"action":  task.Action,
	}, proxy)
	if err != nil || resp == nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}

	ti, err := adaptor.ParseTaskResult(body)
	if err != nil || ti == nil {
		return nil
	}

	snap := task.Snapshot()

	// 将上游最新状态更新到 task
	if ti.Status != "" {
		task.Status = model.TaskStatus(ti.Status)
	}
	if ti.Progress != "" {
		task.Progress = ti.Progress
	}
	if strings.HasPrefix(ti.Url, "data:") {
		// data: URI — kept in Data, not ResultURL
	} else if ti.Url != "" {
		task.PrivateData.ResultURL = ti.Url
	} else if task.Status == model.TaskStatusSuccess {
		// No URL from adaptor — construct proxy URL using public task ID
		task.PrivateData.ResultURL = taskcommon.BuildProxyURL(task.TaskID)
	}

	if !snap.Equal(task.Snapshot()) {
		_, _ = task.UpdateWithStatus(snap.Status)
	}

	// OpenAI Video API 由调用者的 ConvertToOpenAIVideo 分支处理
	if isOpenAIVideoAPI {
		return nil
	}

	// 非 OpenAI Video API: 构建自定义格式响应
	format := detectVideoFormat(body)
	out := map[string]any{
		"error":    nil,
		"format":   format,
		"metadata": nil,
		"status":   mapTaskStatusToSimple(task.Status),
		"task_id":  task.TaskID,
		"url":      task.GetResultURL(),
	}
	respBody, _ := common.Marshal(dto.TaskResponse[any]{
		Code: "success",
		Data: out,
	})
	return respBody
}

// detectVideoFormat 从 Gemini/Vertex 原始响应中探测视频格式
func detectVideoFormat(rawBody []byte) string {
	var raw map[string]any
	if err := common.Unmarshal(rawBody, &raw); err != nil {
		return "mp4"
	}
	respObj, ok := raw["response"].(map[string]any)
	if !ok {
		return "mp4"
	}
	vids, ok := respObj["videos"].([]any)
	if !ok || len(vids) == 0 {
		return "mp4"
	}
	v0, ok := vids[0].(map[string]any)
	if !ok {
		return "mp4"
	}
	mt, ok := v0["mimeType"].(string)
	if !ok || mt == "" || strings.Contains(mt, "mp4") {
		return "mp4"
	}
	return mt
}

// mapTaskStatusToSimple 将内部 TaskStatus 映射为简化状态字符串
func mapTaskStatusToSimple(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	case model.TaskStatusQueued, model.TaskStatusSubmitted:
		return "queued"
	default:
		return "processing"
	}
}

func TaskModel2Dto(task *model.Task) *dto.TaskDto {
	return &dto.TaskDto{
		ID:         task.ID,
		CreatedAt:  task.CreatedAt,
		UpdatedAt:  task.UpdatedAt,
		TaskID:     task.TaskID,
		Platform:   string(task.Platform),
		UserId:     task.UserId,
		Group:      task.Group,
		ChannelId:  task.ChannelId,
		Quota:      task.Quota,
		Action:     task.Action,
		Status:     string(task.Status),
		FailReason: task.FailReason,
		ResultURL:  task.GetResultURL(),
		SubmitTime: task.SubmitTime,
		StartTime:  task.StartTime,
		FinishTime: task.FinishTime,
		Progress:   task.Progress,
		Properties: task.Properties,
		Username:   task.Username,
		Data:       task.Data,
	}
}

// imageTaskFetchByIDRespBodyBuilder handles GET /v1/images/tasks/:task_id
// Returns image task status in the image.task format defined in the design doc.
func imageTaskFetchByIDRespBodyBuilder(c *gin.Context) (respBody []byte, taskResp *dto.TaskError) {
	taskId := c.Param("task_id")
	if taskId == "" {
		taskId = c.GetString("task_id")
	}
	userId := c.GetInt("id")

	originTask, exist, err := model.GetByTaskId(userId, taskId)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "get_task_failed", http.StatusInternalServerError)
		return
	}
	if !exist {
		taskResp = service.TaskErrorWrapperLocal(errors.New("task_not_exist"), "task_not_exist", http.StatusBadRequest)
		return
	}

	resp := map[string]any{
		"id":      originTask.TaskID,
		"object":  "image.task",
		"status":  mapTaskStatusToImageStatus(originTask.Status),
		"model":   originTask.Properties.OriginModelName,
		"created": originTask.CreatedAt,
	}

	switch originTask.Status {
	case model.TaskStatusSuccess:
		resultURL := originTask.GetResultURL()
		var imageData []map[string]string
		for _, u := range strings.Split(resultURL, ",") {
			u = strings.TrimSpace(u)
			if u != "" {
				imageData = append(imageData, map[string]string{"url": u})
			}
		}
		resp["result"] = map[string]any{"data": imageData}
	case model.TaskStatusFailure:
		resp["error"] = map[string]string{
			"message": originTask.FailReason,
			"code":    "task_failed",
		}
	}

	respBody, err = common.Marshal(resp)
	if err != nil {
		taskResp = service.TaskErrorWrapper(err, "marshal_response_failed", http.StatusInternalServerError)
	}
	return
}

// mapTaskStatusToImageStatus maps internal TaskStatus to image task status string
func mapTaskStatusToImageStatus(status model.TaskStatus) string {
	switch status {
	case model.TaskStatusSuccess:
		return "succeeded"
	case model.TaskStatusFailure:
		return "failed"
	default:
		return "processing"
	}
}
