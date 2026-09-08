package controller

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/logger"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"
	"github.com/gin-gonic/gin"
)

// ============================================================
// helpers
// ============================================================

func seedanceGetGW(c *gin.Context) (*service.SeedanceGatewayChannel, bool) {
	userGroup := c.GetString("group")
	gw, err := service.GetSeedanceGatewayChannel(userGroup)
	if err != nil {
		logger.LogError(c, fmt.Sprintf("seedance: no gateway channel for group %s: %v", userGroup, err))
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": err.Error()})
		return nil, false
	}
	return gw, true
}

// proxyAndPassthrough sends the request to upstream and writes response back.
// It also calls the onSuccess callback (with status code and body) before writing.
// If the callback writes a response, proxyAndPassthrough will not write again.
func proxyAndPassthrough(c *gin.Context, gw *service.SeedanceGatewayChannel, method, path string, query url.Values, body []byte, onSuccess func(statusCode int, body []byte)) {
	statusCode, respBody, err := service.AssetProxyRequest(gw, method, path, query, body)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}
	if statusCode < 200 || statusCode >= 300 {
		c.JSON(statusCode, gin.H{"success": false, "message": extractUpstreamErrMsg(respBody)})
		return
	}
	if onSuccess != nil {
		onSuccess(statusCode, respBody)
	}
	// 检查回调是否已经写入响应，如果是则不再写入
	if !c.Writer.Written() {
		c.Data(statusCode, "application/json; charset=utf-8", respBody)
	}
}

// extractUpstreamErrMsg 从上游标准错误结构中提取 Message，解析失败时返回通用提示。
func extractUpstreamErrMsg(body []byte) string {
	var errResp struct {
		ResponseMetadata struct {
			Error struct {
				Message string `json:"Message"`
			} `json:"Error"`
		} `json:"ResponseMetadata"`
	}
	if common.Unmarshal(body, &errResp) == nil && errResp.ResponseMetadata.Error.Message != "" {
		return errResp.ResponseMetadata.Error.Message
	}
	return "upstream request failed"
}

func readBody(c *gin.Context) []byte {
	body, _ := c.GetRawData()
	return body
}

func forwardQuery(c *gin.Context) url.Values {
	return c.Request.URL.Query()
}

// ============================================================
// Asset Groups
// ============================================================

// POST /api/seedance/asset-groups
// 使用适配器模式重构版本
func SeedanceCreateAssetGroup(c *gin.Context) {
	userID := c.GetInt("id")
	userGroup := c.GetString("group")

	// 获取适配器
	adapter, channel, err := service.GetAssetAdapter(userGroup)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": err.Error()})
		return
	}

	// 解析请求体
	var req service.CreateAssetGroupRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request"})
		return
	}

	// 使用适配器创建素材组
	resp, err := adapter.CreateAssetGroup(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}

	// 保存到数据库
	g := &model.SeedanceAssetGroup{
		UserID:          userID,
		ChannelID:       channel.Id,
		UpstreamGroupID: resp.ID,
		Name:            resp.Name,
		Description:     resp.Description,
		GroupType:       resp.GroupType,
		RawData:         resp.RawData,
	}
	_ = model.CreateSeedanceAssetGroup(g)

	// 返回响应（兼容旧格式，包装在 Result 中）
	result := map[string]interface{}{
		"Id":          resp.ID,
		"LocalId":     resp.ID,
		"Name":        resp.Name,
		"Description": resp.Description,
		"GroupType":   resp.GroupType,
	}
	c.JSON(http.StatusOK, gin.H{"Result": result})
}

// GET /api/seedance/asset-groups
func SeedanceListAssetGroups(c *gin.Context) {
	userID := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	groups, total, err := model.ListSeedanceAssetGroups(userID, pageInfo.GetPage(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 检查是否需要返回火山官方格式（通过 RelayAsset 调用）
	if c.GetBool("doubao_official_format") {
		// 转换为火山官方格式
		items := []map[string]interface{}{}
		for _, g := range groups {
			item := map[string]interface{}{
				"Id":          g.UpstreamGroupID,
				"Name":        g.Name,
				"GroupType":   g.GroupType,
				"CreateTime":  formatTime(g.CreatedAt),
				"UpdateTime":  formatTime(g.UpdatedAt),
			}
			if g.Description != "" {
				item["Description"] = g.Description
			}
			items = append(items, item)
		}

		c.JSON(http.StatusOK, gin.H{
			"ResponseMetadata": gin.H{
				"RequestId": generateRequestID(),
				"Action":    "ListAssetGroups",
				"Version":   "2024-01-01",
				"Service":   "ark",
				"Region":    "cn-beijing",
			},
			"Result": gin.H{
				"Items":      items,
				"TotalCount": total,
				"PageNumber": pageInfo.GetPage(),
				"PageSize":   pageInfo.GetPageSize(),
			},
		})
		return
	}

	// 默认返回本地格式（RESTful API）
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(groups)
	common.ApiSuccess(c, pageInfo)
}

// GET /api/seedance/asset-groups/:id
// 使用适配器模式重构版本
func SeedanceGetAssetGroup(c *gin.Context) {
	userID := c.GetInt("id")
	userGroup := c.GetString("group")

	// 获取适配器
	adapter, channel, err := service.GetAssetAdapter(userGroup)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": err.Error()})
		return
	}

	// 解析素材组 ID
	g, err := resolveAssetGroup(c, userID, channel.Id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "asset group not found"})
		return
	}

	// 使用适配器查询素材组
	resp, err := adapter.GetAssetGroup(g.UpstreamGroupID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}

	// 返回响应（兼容旧格式，包装在 Result 中）
	result := map[string]interface{}{
		"Id":          resp.ID,
		"Name":        resp.Name,
		"Description": resp.Description,
		"GroupType":   resp.GroupType,
	}
	c.JSON(http.StatusOK, gin.H{"Result": result})
}

// PUT /api/seedance/asset-groups/:id
func SeedancePutAssetGroup(c *gin.Context) {
	seedanceModifyAssetGroup(c)
}

// PATCH /api/seedance/asset-groups/:id
func SeedancePatchAssetGroup(c *gin.Context) {
	seedanceModifyAssetGroup(c)
}

// 使用适配器模式重构版本
func seedanceModifyAssetGroup(c *gin.Context) {
	userID := c.GetInt("id")
	userGroup := c.GetString("group")

	// 获取适配器
	adapter, channel, err := service.GetAssetAdapter(userGroup)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": err.Error()})
		return
	}

	// 解析素材组 ID
	g, err := resolveAssetGroup(c, userID, channel.Id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "asset group not found"})
		return
	}

	// 解析请求体
	var req service.UpdateAssetGroupRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request"})
		return
	}

	// 使用适配器更新素材组
	if err := adapter.UpdateAssetGroup(g.UpstreamGroupID, req); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}

	// 更新本地数据库
	_ = model.UpdateSeedanceAssetGroupRaw(g.ID, req.Name, req.Description, "")

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// DELETE /api/seedance/asset-groups/:id
// 使用适配器模式重构版本
func SeedanceDeleteAssetGroup(c *gin.Context) {
	userID := c.GetInt("id")
	userGroup := c.GetString("group")

	// 获取适配器
	adapter, channel, err := service.GetAssetAdapter(userGroup)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": err.Error()})
		return
	}

	// 解析素材组 ID
	g, err := resolveAssetGroup(c, userID, channel.Id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "asset group not found"})
		return
	}

	// 使用适配器删除素材组
	if err := adapter.DeleteAssetGroup(g.UpstreamGroupID); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}

	// 软删除本地记录
	_ = model.SoftDeleteSeedanceAssetGroup(g.ID, userID)

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ============================================================
// Assets
// ============================================================

// getOrCreateDefaultAssetGroup 获取或创建用户的默认 AIGC 素材组（每用户每渠道一个）
// 使用适配器模式重构版本
func getOrCreateDefaultAssetGroup(c *gin.Context, userGroup string, userID int) (string, int, error) {
	// 获取适配器
	adapter, channel, err := service.GetAssetAdapter(userGroup)
	if err != nil {
		return "", 0, fmt.Errorf("get asset adapter failed: %w", err)
	}

	// 查本地表有无该用户+渠道的 AIGC 素材组，必须匹配渠道，避免跨渠道引用不存在的 group
	groups, _, err := model.ListSeedanceAssetGroupsByChannel(userID, channel.Id, 1, 1)
	if err == nil && len(groups) > 0 {
		logger.LogInfo(c, fmt.Sprintf("seedance: found existing asset group %s for user %d", groups[0].UpstreamGroupID, userID))
		return groups[0].UpstreamGroupID, channel.Id, nil
	}

	// 没有则创建，组名用 u{id}-{md5(username)前8位}，保证 ASCII 且可追溯
	user, err2 := model.GetUserById(userID, false)
	groupName := fmt.Sprintf("u%d", userID)
	if err2 == nil && user != nil && user.Username != "" {
		h := fmt.Sprintf("%x", md5.Sum([]byte(user.Username)))
		groupName = fmt.Sprintf("u%d-%s", userID, h[:8])
	}

	logger.LogInfo(c, fmt.Sprintf("seedance: creating default asset group '%s' for user %d", groupName, userID))

	// 使用适配器创建素材组
	req := service.CreateAssetGroupRequest{
		Name:        groupName,
		Description: "auto-created",
		GroupType:   "AIGC",
	}

	resp, err3 := adapter.CreateAssetGroup(req)
	if err3 != nil {
		return "", 0, fmt.Errorf("create default asset group failed: %w", err3)
	}

	// 保存到数据库
	g := &model.SeedanceAssetGroup{
		UserID:          userID,
		ChannelID:       channel.Id,
		UpstreamGroupID: resp.ID,
		Name:            resp.Name,
		GroupType:       resp.GroupType,
		RawData:         resp.RawData,
	}
	_ = model.CreateSeedanceAssetGroup(g)
	logger.LogInfo(c, fmt.Sprintf("seedance: created asset group %s for user %d", resp.ID, userID))
	return resp.ID, channel.Id, nil
}

// POST /api/seedance/assets
// 使用适配器模式重构版本
func SeedanceCreateAsset(c *gin.Context) {
	userID := c.GetInt("id")
	userGroup := c.GetString("group")

	// 解析请求体
	var req struct {
		GroupID   string `json:"GroupId"`
		URL       string `json:"URL"`
		AssetType string `json:"AssetType"`
		Name      string `json:"Name"`
		Force     bool   `json:"Force"` // true=强制重新上传，忽略本地缓存
		Model     string `json:"Model"` // 可选：指定模型名，用于选择对应渠道
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request"})
		return
	}

	// 获取适配器（支持根据模型名选择渠道）
	adapter, channel, err := service.GetAssetAdapterByModel(userGroup, req.Model)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": err.Error()})
		return
	}

	// 如果未强制，先按 source_url 查本地表，有 Active 记录直接返回
	if !req.Force && req.URL != "" {
		if existing, err := model.GetSeedanceAssetBySourceURL(req.URL, userID, channel.Id); err == nil && existing.Status == "Active" {
			logger.LogInfo(c, fmt.Sprintf("seedance: asset already exists for url %s, asset_id=%s", req.URL, existing.UpstreamAssetID))
			result := map[string]interface{}{
				"Id":       existing.UpstreamAssetID,
				"LocalId":  existing.ID,
				"AssetRef": "asset://" + existing.UpstreamAssetID,
				"Status":   existing.Status,
			}
			c.JSON(http.StatusOK, gin.H{"Result": result})
			return
		}
	}

	// 如果没有指定 GroupID，自动创建默认素材组
	if req.GroupID == "" {
		groupID, channelID, err := getOrCreateDefaultAssetGroup(c, userGroup, userID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": err.Error()})
			return
		}
		req.GroupID = groupID
		// 确保使用同一个渠道
		if channel.Id != channelID {
			logger.LogWarn(c, fmt.Sprintf("channel mismatch: adapter=%d, default_group=%d", channel.Id, channelID))
		}
	}

	// 使用适配器创建素材
	createReq := service.CreateAssetRequest{
		GroupID:   req.GroupID,
		URL:       req.URL,
		AssetType: req.AssetType,
		Name:      req.Name,
	}

	resp, err := adapter.CreateAsset(createReq)
	if err != nil {
		// 若上游返回 group_id 不存在，自动软删本地旧记录并重建 group 后重试一次
		if strings.Contains(err.Error(), "NotFound.group_id") || strings.Contains(err.Error(), "group") {
			logger.LogWarn(c, fmt.Sprintf("seedance: upstream group %s not found, rebuilding for user %d", req.GroupID, userID))
			_ = model.SoftDeleteSeedanceAssetGroupByUpstreamID(req.GroupID, userID)

			newGroupID, _, rebuildErr := getOrCreateDefaultAssetGroup(c, userGroup, userID)
			if rebuildErr != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": rebuildErr.Error()})
				return
			}

			// 重试
			createReq.GroupID = newGroupID
			resp, err = adapter.CreateAsset(createReq)
			if err != nil {
				c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
				return
			}
		} else {
			c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
			return
		}
	}

	// 保存到数据库
	a := &model.SeedanceAsset{
		UserID:          userID,
		ChannelID:       channel.Id,
		UpstreamAssetID: resp.ID,
		UpstreamGroupID: resp.GroupID,
		Name:            resp.Name,
		AssetType:       resp.AssetType,
		SourceURL:       req.URL,
		Status:          resp.Status,
		RawData:         resp.RawData,
	}
	_ = model.CreateSeedanceAsset(a)

	// 返回响应（兼容旧格式，包装在 Result 中）
	result := map[string]interface{}{
		"Id":       resp.ID,
		"LocalId":  resp.ID,
		"AssetRef": "asset://" + resp.ID,
		"Status":   resp.Status,
		"GroupId":  resp.GroupID,
	}
	c.JSON(http.StatusOK, gin.H{"Result": result})
}

// GET /api/seedance/assets
func SeedanceListAssets(c *gin.Context) {
	userID := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	groupID := c.Query("group_id")
	assetID, _ := strconv.ParseInt(c.Query("id"), 10, 64)
	upstreamAssetID := strings.TrimSpace(c.Query("upstream_asset_id"))
	assets, total, err := model.ListSeedanceAssets(userID, groupID, assetID, upstreamAssetID, pageInfo.GetPage(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}

	// 检查是否需要返回火山官方格式（通过 RelayAsset 调用）
	if c.GetBool("doubao_official_format") {
		// 转换为火山官方格式
		items := []map[string]interface{}{}
		for _, a := range assets {
			item := map[string]interface{}{
				"Id":          a.UpstreamAssetID,
				"GroupId":     a.UpstreamGroupID,
				"Name":        a.Name,
				"AssetType":   a.AssetType,
				"Status":      a.Status,
				"CreateTime":  formatTime(a.CreatedAt),
				"UpdateTime":  formatTime(a.UpdatedAt),
			}
			if a.SourceURL != "" {
				item["URL"] = a.SourceURL
			}
			items = append(items, item)
		}

		c.JSON(http.StatusOK, gin.H{
			"ResponseMetadata": gin.H{
				"RequestId": generateRequestID(),
				"Action":    "ListAssets",
				"Version":   "2024-01-01",
				"Service":   "ark",
				"Region":    "cn-beijing",
			},
			"Result": gin.H{
				"Items":      items,
				"TotalCount": total,
				"PageNumber": pageInfo.GetPage(),
				"PageSize":   pageInfo.GetPageSize(),
			},
		})
		return
	}

	// 默认返回本地格式（RESTful API）
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(assets)
	common.ApiSuccess(c, pageInfo)
}

// resolveAsset 支持本地数字 ID 或上游 asset-xxxxxxxx 格式查询素材
func resolveAsset(c *gin.Context, userID int, channelID int) (*model.SeedanceAsset, error) {
	rawID := c.Param("id")
	if strings.HasPrefix(rawID, "asset-") {
		return model.GetSeedanceAssetByUpstreamID(rawID, userID, channelID)
	}
	localID, _ := strconv.ParseInt(rawID, 10, 64)
	return model.GetSeedanceAssetByID(localID, userID)
}

// resolveAssetGroup 支持本地数字 ID 或上游 group-xxxxxxxx 格式查询素材组
func resolveAssetGroup(c *gin.Context, userID int, channelID int) (*model.SeedanceAssetGroup, error) {
	rawID := c.Param("id")
	if strings.HasPrefix(rawID, "group-") {
		return model.GetSeedanceAssetGroupByUpstreamID(rawID, userID, channelID)
	}
	localID, _ := strconv.ParseInt(rawID, 10, 64)
	return model.GetSeedanceAssetGroupByID(localID, userID)
}

// resolveFaceVerification 支持本地数字 ID 或上游 fv_xxxxxxxxx 格式查询人脸认证任务
func resolveFaceVerification(c *gin.Context, userID int) (*model.SeedanceFaceVerification, error) {
	rawID := c.Param("id")
	if strings.HasPrefix(rawID, "fv_") {
		return model.GetSeedanceFaceVerificationByVerificationID(rawID, userID)
	}
	localID, _ := strconv.ParseInt(rawID, 10, 64)
	return model.GetSeedanceFaceVerificationByID(localID, userID)
}

// GET /api/seedance/assets/:id
// 使用适配器模式重构版本
func SeedanceGetAsset(c *gin.Context) {
	userID := c.GetInt("id")
	userGroup := c.GetString("group")

	// 获取适配器
	adapter, channel, err := service.GetAssetAdapter(userGroup)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": err.Error()})
		return
	}

	// 解析素材 ID（支持本地 ID 或上游 ID）
	a, err := resolveAsset(c, userID, channel.Id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "asset not found"})
		return
	}

	// 使用适配器查询素材
	resp, err := adapter.GetAsset(a.UpstreamAssetID, forwardQuery(c))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}

	// 更新本地数据库状态
	if resp.Status != "" {
		_ = model.UpdateSeedanceAssetStatus(a.ID, resp.Status, resp.RawData)
	}

	// 返回响应（兼容旧格式，包装在 Result 中）
	result := map[string]interface{}{
		"Id":        resp.ID,
		"Status":    resp.Status,
		"Name":      resp.Name,
		"GroupId":   resp.GroupID,
		"AssetType": resp.AssetType,
	}
	c.JSON(http.StatusOK, gin.H{"Result": result})
}

// PUT /api/seedance/assets/:id
func SeedancePutAsset(c *gin.Context) {
	seedanceModifyAsset(c)
}

// PATCH /api/seedance/assets/:id
func SeedancePatchAsset(c *gin.Context) {
	seedanceModifyAsset(c)
}

// 使用适配器模式重构版本
func seedanceModifyAsset(c *gin.Context) {
	userID := c.GetInt("id")
	userGroup := c.GetString("group")

	// 获取适配器
	adapter, channel, err := service.GetAssetAdapter(userGroup)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": err.Error()})
		return
	}

	// 解析素材 ID
	a, err := resolveAsset(c, userID, channel.Id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "asset not found"})
		return
	}

	// 解析请求体
	var req service.UpdateAssetRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"success": false, "message": "invalid request"})
		return
	}

	// 使用适配器更新素材
	if err := adapter.UpdateAsset(a.UpstreamAssetID, req); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}

	// 更新本地数据库
	_ = model.UpdateSeedanceAssetStatus(a.ID, a.Status, "")

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// DELETE /api/seedance/assets/:id
// 使用适配器模式重构版本
func SeedanceDeleteAsset(c *gin.Context) {
	userID := c.GetInt("id")
	userGroup := c.GetString("group")

	// 获取适配器
	adapter, channel, err := service.GetAssetAdapter(userGroup)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"success": false, "message": err.Error()})
		return
	}

	// 解析素材 ID
	a, err := resolveAsset(c, userID, channel.Id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "asset not found"})
		return
	}

	// 使用适配器删除素材
	if err := adapter.DeleteAsset(a.UpstreamAssetID); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"success": false, "message": err.Error()})
		return
	}

	// 软删除本地记录
	_ = model.SoftDeleteSeedanceAsset(a.ID, userID)

	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ============================================================
// Face Verifications
// ============================================================

// POST /api/seedance/face-verifications
func SeedanceCreateFaceVerification(c *gin.Context) {
	gw, ok := seedanceGetGW(c)
	if !ok {
		return
	}
	userID := c.GetInt("id")
	body := readBody(c)

	proxyAndPassthrough(c, gw, http.MethodPost, "/api/seedance/face-verifications", nil, body, func(_ int, respBody []byte) {
		var resp struct {
			VerificationID string `json:"verification_id"`
			H5URL          string `json:"h5_url"`
			ExpiresAt      int64  `json:"expires_at"`
		}
		if err := common.Unmarshal(respBody, &resp); err != nil || resp.VerificationID == "" {
			return
		}
		v := &model.SeedanceFaceVerification{
			UserID:         userID,
			ChannelID:      gw.Channel.Id,
			VerificationID: resp.VerificationID,
			Status:         "waiting_user",
			H5URL:          resp.H5URL,
			ExpiresAt:      resp.ExpiresAt,
			RawData:        string(respBody),
		}
		_ = model.CreateSeedanceFaceVerification(v)
	})
}

// GET /api/seedance/face-verifications/:id
func SeedanceGetFaceVerification(c *gin.Context) {
	gw, ok := seedanceGetGW(c)
	if !ok {
		return
	}
	userID := c.GetInt("id")
	v, err := resolveFaceVerification(c, userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"success": false, "message": "face verification not found"})
		return
	}
	proxyAndPassthrough(c, gw, http.MethodGet, "/api/seedance/face-verifications/"+v.VerificationID, nil, nil, func(_ int, respBody []byte) {
		var resp struct {
			Status  string `json:"status"`
			GroupID string `json:"group_id"`
		}
		if err2 := common.Unmarshal(respBody, &resp); err2 == nil && resp.Status != "" {
			_ = model.UpdateSeedanceFaceVerificationStatus(v.ID, resp.Status, resp.GroupID, string(respBody))
		}
	})
}

// ============================================================
// Admin list endpoints (GET /api/admin/seedance/*)
// ============================================================

// GET /api/admin/seedance/asset-groups
func SeedanceAdminListAssetGroups(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userIDFilter, _ := strconv.Atoi(c.Query("user_id"))
	groups, total, err := model.ListAllSeedanceAssetGroups(pageInfo.GetPage(), pageInfo.GetPageSize(), userIDFilter)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(groups)
	common.ApiSuccess(c, pageInfo)
}

// GET /api/admin/seedance/assets
func SeedanceAdminListAssets(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userIDFilter, _ := strconv.Atoi(c.Query("user_id"))
	groupID := c.Query("group_id")
	assetID, _ := strconv.ParseInt(c.Query("id"), 10, 64)
	upstreamAssetID := strings.TrimSpace(c.Query("upstream_asset_id"))
	assets, total, err := model.ListAllSeedanceAssets(pageInfo.GetPage(), pageInfo.GetPageSize(), userIDFilter, groupID, assetID, upstreamAssetID)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(assets)
	common.ApiSuccess(c, pageInfo)
}

// GET /api/seedance/face-verifications
func SeedanceListFaceVerifications(c *gin.Context) {
	userID := c.GetInt("id")
	pageInfo := common.GetPageQuery(c)
	items, total, err := model.ListSeedanceFaceVerifications(userID, pageInfo.GetPage(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// GET /api/admin/seedance/face-verifications
func SeedanceAdminListFaceVerifications(c *gin.Context) {
	pageInfo := common.GetPageQuery(c)
	userIDFilter, _ := strconv.Atoi(c.Query("user_id"))
	items, total, err := model.ListAllSeedanceFaceVerifications(pageInfo.GetPage(), pageInfo.GetPageSize(), userIDFilter)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(items)
	common.ApiSuccess(c, pageInfo)
}

// ensure json import used
var _ = json.Marshal

// formatTime 将 Unix 时间戳转换为 ISO 8601 格式
func formatTime(timestamp int64) string {
	if timestamp == 0 {
		return ""
	}
	return fmt.Sprintf("%s", time.Unix(timestamp, 0).UTC().Format(time.RFC3339))
}

// generateRequestID 生成火山格式的请求 ID
func generateRequestID() string {
	// 格式：YYYYMMDDHHMMSS + 随机字符串
	now := time.Now()
	randomPart := fmt.Sprintf("%X", md5.Sum([]byte(fmt.Sprintf("%d%d", now.UnixNano(), rand.Int()))))
	return fmt.Sprintf("%s%s", now.Format("20060102150405"), randomPart[:20])
}
