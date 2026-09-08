import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  createColumnHelper,
  flexRender,
  getCoreRowModel,
  useReactTable,
} from '@tanstack/react-table'
import { SectionPageLayout } from '@/components/layout'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { useAuthStore } from '@/stores/auth-store'
import { ROLE } from '@/lib/roles'
import {
  adminListAssetGroups,
  adminListAssets,
  adminListFaceVerifications,
  userListAssetGroups,
  userListAssets,
  userListFaceVerifications,
  seedanceQueryKeys,
} from './api'
import type {
  SeedanceAsset,
  SeedanceAssetGroup,
  SeedanceFaceVerification,
} from './types'

function formatTime(ts: number) {
  if (!ts) return '—'
  return new Date(ts * 1000).toLocaleString()
}

// ---- Asset Groups Table ----

const groupColHelper = createColumnHelper<SeedanceAssetGroup>()

function AssetGroupsTab() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const pageSize = 20
  const isAdmin = (useAuthStore.getState().auth.user?.role ?? 0) >= ROLE.ADMIN

  const { data, isLoading } = useQuery({
    queryKey: seedanceQueryKeys.assetGroups({ p: page, page_size: pageSize }),
    queryFn: () =>
      isAdmin
        ? adminListAssetGroups({ p: page, page_size: pageSize })
        : userListAssetGroups({ p: page, page_size: pageSize }),
  })

  const columns = [
    groupColHelper.accessor('id', { header: 'ID', size: 60 }),
    ...(isAdmin
      ? [groupColHelper.accessor('user_id', { header: t('User ID'), size: 80 })]
      : []),
    groupColHelper.accessor('upstream_group_id', {
      header: t('Upstream Group ID'),
      cell: (info) => (
        <span className='font-mono text-xs'>{info.getValue()}</span>
      ),
    }),
    groupColHelper.accessor('name', { header: t('Name') }),
    groupColHelper.accessor('group_type', {
      header: t('Type'),
      cell: (info) => (
        <Badge variant={info.getValue() === 'AIGC' ? 'default' : 'secondary'}>
          {info.getValue()}
        </Badge>
      ),
      size: 120,
    }),
    groupColHelper.accessor('created_at', {
      header: t('Created At'),
      cell: (info) => (
        <span className='text-muted-foreground text-xs'>
          {formatTime(info.getValue())}
        </span>
      ),
    }),
  ]

  const table = useReactTable({
    data: data?.items ?? [],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  const total = data?.total ?? 0
  const totalPages = Math.ceil(total / pageSize)

  return (
    <div className='space-y-3'>
      <DataTable table={table} columns={columns} isLoading={isLoading} />
      <Pagination page={page} totalPages={totalPages} onPageChange={setPage} total={total} />
    </div>
  )
}

// ---- Assets Table ----

const assetColHelper = createColumnHelper<SeedanceAsset>()

function AssetsTab() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const [idInput, setIdInput] = useState('')
  const [upstreamAssetIdInput, setUpstreamAssetIdInput] = useState('')
  const [filters, setFilters] = useState<{ id?: number; upstream_asset_id?: string }>({})
  const pageSize = 20
  const isAdmin = (useAuthStore.getState().auth.user?.role ?? 0) >= ROLE.ADMIN
  const queryParams = { p: page, page_size: pageSize, ...filters }

  const { data, isLoading } = useQuery({
    queryKey: seedanceQueryKeys.assets(queryParams),
    queryFn: () =>
      isAdmin
        ? adminListAssets(queryParams)
        : userListAssets(queryParams),
  })

  const columns = [
    assetColHelper.accessor('id', { header: 'ID', size: 60 }),
    ...(isAdmin
      ? [
          assetColHelper.accessor('user_id', { header: t('User ID'), size: 80 }),
          assetColHelper.accessor('channel_id', { header: t('Channel ID'), size: 80 }),
        ]
      : []),
    assetColHelper.accessor('upstream_asset_id', {
      header: t('Upstream Asset ID'),
      cell: (info) => (
        <span className='font-mono text-xs'>{info.getValue()}</span>
      ),
    }),
    assetColHelper.accessor('upstream_group_id', {
      header: t('Group ID'),
      cell: (info) => (
        <span className='font-mono text-xs text-muted-foreground'>
          {info.getValue()}
        </span>
      ),
    }),
    assetColHelper.accessor('name', { header: t('Name') }),
    assetColHelper.accessor('source_url', {
      header: t('URL'),
      cell: (info) => {
        const url = info.getValue()
        if (!url) return <span className='text-muted-foreground text-xs'>—</span>
        const isImage = info.row.original.asset_type === 'Image'
        return (
          <a
            href={url}
            target='_blank'
            rel='noopener noreferrer'
            title={url}
            className='inline-flex items-center gap-1 text-xs text-blue-500 hover:underline max-w-[180px] truncate block'
          >
            {isImage ? '🖼 ' : ''}
            {url.split('/').pop() ?? url}
          </a>
        )
      },
    }),
    assetColHelper.accessor('asset_type', {
      header: t('Type'),
      cell: (info) => <Badge variant='outline'>{info.getValue()}</Badge>,
      size: 90,
    }),
    assetColHelper.accessor('status', {
      header: t('Status'),
      cell: (info) => {
        const s = info.getValue()
        const variant =
          s === 'Active'
            ? 'default'
            : s === 'Failed'
              ? 'destructive'
              : 'secondary'
        if (s === 'Failed') {
          const rawData = info.row.original.raw_data
          return (
            <Popover>
              <PopoverTrigger asChild>
                <Badge variant={variant} className='cursor-pointer'>
                  {s}
                </Badge>
              </PopoverTrigger>
              <PopoverContent className='w-96 max-h-64 overflow-auto'>
                <pre className='text-xs whitespace-pre-wrap break-all'>
                  {rawData || t('No detail available')}
                </pre>
              </PopoverContent>
            </Popover>
          )
        }
        return <Badge variant={variant}>{s}</Badge>
      },
      size: 110,
    }),
    assetColHelper.accessor('created_at', {
      header: t('Created At'),
      cell: (info) => (
        <span className='text-muted-foreground text-xs'>
          {formatTime(info.getValue())}
        </span>
      ),
    }),
  ]

  const table = useReactTable({
    data: data?.items ?? [],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  const total = data?.total ?? 0
  const totalPages = Math.ceil(total / pageSize)

  const handleSearch = () => {
    const nextFilters: { id?: number; upstream_asset_id?: string } = {}
    const id = Number(idInput.trim())
    if (Number.isInteger(id) && id > 0) {
      nextFilters.id = id
    }
    const upstreamAssetId = upstreamAssetIdInput.trim()
    if (upstreamAssetId) {
      nextFilters.upstream_asset_id = upstreamAssetId
    }
    setFilters(nextFilters)
    setPage(1)
  }

  return (
    <div className='space-y-3'>
      <div className='flex flex-col gap-2 sm:flex-row sm:items-center'>
        <Input
          value={idInput}
          onChange={(event) => setIdInput(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') handleSearch()
          }}
          placeholder={t('Asset ID')}
          className='sm:w-40'
        />
        <Input
          value={upstreamAssetIdInput}
          onChange={(event) => setUpstreamAssetIdInput(event.target.value)}
          onKeyDown={(event) => {
            if (event.key === 'Enter') handleSearch()
          }}
          placeholder={t('Upstream Asset ID')}
          className='sm:w-72'
        />
        <Button onClick={handleSearch}>{t('Search')}</Button>
      </div>
      <DataTable table={table} columns={columns} isLoading={isLoading} />
      <Pagination page={page} totalPages={totalPages} onPageChange={setPage} total={total} />
    </div>
  )
}

// ---- Face Verifications Table ----

const faceColHelper = createColumnHelper<SeedanceFaceVerification>()

function FaceVerificationsTab() {
  const { t } = useTranslation()
  const [page, setPage] = useState(1)
  const pageSize = 20
  const isAdmin = (useAuthStore.getState().auth.user?.role ?? 0) >= ROLE.ADMIN

  const { data, isLoading } = useQuery({
    queryKey: seedanceQueryKeys.faceVerifications({ p: page, page_size: pageSize }),
    queryFn: () =>
      isAdmin
        ? adminListFaceVerifications({ p: page, page_size: pageSize })
        : userListFaceVerifications({ p: page, page_size: pageSize }),
  })

  const columns = [
    faceColHelper.accessor('id', { header: 'ID', size: 60 }),
    ...(isAdmin
      ? [faceColHelper.accessor('user_id', { header: t('User ID'), size: 80 })]
      : []),
    faceColHelper.accessor('verification_id', {
      header: t('Verification ID'),
      cell: (info) => (
        <span className='font-mono text-xs'>{info.getValue()}</span>
      ),
    }),
    faceColHelper.accessor('status', {
      header: t('Status'),
      cell: (info) => {
        const s = info.getValue()
        const variant =
          s === 'verified'
            ? 'default'
            : s === 'failed' || s === 'expired'
              ? 'destructive'
              : 'secondary'
        return <Badge variant={variant}>{s}</Badge>
      },
      size: 130,
    }),
    faceColHelper.accessor('group_id', {
      header: t('Group ID'),
      cell: (info) => (
        <span className='font-mono text-xs text-muted-foreground'>
          {info.getValue() || '—'}
        </span>
      ),
    }),
    faceColHelper.accessor('expires_at', {
      header: t('Expires At'),
      cell: (info) => (
        <span className='text-muted-foreground text-xs'>
          {formatTime(info.getValue())}
        </span>
      ),
    }),
    faceColHelper.accessor('created_at', {
      header: t('Created At'),
      cell: (info) => (
        <span className='text-muted-foreground text-xs'>
          {formatTime(info.getValue())}
        </span>
      ),
    }),
  ]

  const table = useReactTable({
    data: data?.items ?? [],
    columns,
    getCoreRowModel: getCoreRowModel(),
  })

  const total = data?.total ?? 0
  const totalPages = Math.ceil(total / pageSize)

  return (
    <div className='space-y-3'>
      <DataTable table={table} columns={columns} isLoading={isLoading} />
      <Pagination page={page} totalPages={totalPages} onPageChange={setPage} total={total} />
    </div>
  )
}

// ---- Shared DataTable ----

function DataTable<T>({
  table,
  columns,
  isLoading,
}: {
  table: ReturnType<typeof useReactTable<T>>
  columns: unknown[]
  isLoading: boolean
}) {
  const { t } = useTranslation()

  if (isLoading) {
    return (
      <div className='text-muted-foreground py-8 text-center'>
        {t('Loading...')}
      </div>
    )
  }

  return (
    <div className='rounded-md border'>
      <Table>
        <TableHeader>
          {table.getHeaderGroups().map((hg) => (
            <TableRow key={hg.id}>
              {hg.headers.map((h) => (
                <TableHead key={h.id} style={{ width: h.getSize() }}>
                  {h.isPlaceholder
                    ? null
                    : flexRender(h.column.columnDef.header, h.getContext())}
                </TableHead>
              ))}
            </TableRow>
          ))}
        </TableHeader>
        <TableBody>
          {table.getRowModel().rows.length ? (
            table.getRowModel().rows.map((row) => (
              <TableRow key={row.id}>
                {row.getVisibleCells().map((cell) => (
                  <TableCell key={cell.id}>
                    {flexRender(cell.column.columnDef.cell, cell.getContext())}
                  </TableCell>
                ))}
              </TableRow>
            ))
          ) : (
            <TableRow>
              <TableCell
                colSpan={columns.length}
                className='text-muted-foreground h-24 text-center'
              >
                {t('No data')}
              </TableCell>
            </TableRow>
          )}
        </TableBody>
      </Table>
    </div>
  )
}

// ---- Pagination ----

function Pagination({
  page,
  totalPages,
  onPageChange,
  total,
}: {
  page: number
  totalPages: number
  onPageChange: (p: number) => void
  total: number
}) {
  const { t } = useTranslation()
  if (totalPages <= 1) return null
  return (
    <div className='flex items-center justify-between'>
      <span className='text-muted-foreground text-sm'>
        {t('Total')}: {total}
      </span>
      <div className='flex gap-2'>
        <Button
          variant='outline'
          size='sm'
          disabled={page <= 1}
          onClick={() => onPageChange(page - 1)}
        >
          {t('Previous')}
        </Button>
        <span className='text-muted-foreground flex items-center text-sm'>
          {page} / {totalPages}
        </span>
        <Button
          variant='outline'
          size='sm'
          disabled={page >= totalPages}
          onClick={() => onPageChange(page + 1)}
        >
          {t('Next')}
        </Button>
      </div>
    </div>
  )
}

// ---- Main Page ----

export function SeedanceAssets() {
  const { t } = useTranslation()

  return (
    <SectionPageLayout>
      <SectionPageLayout.Title>
        {t('Seedance Asset Library')}
      </SectionPageLayout.Title>
      <SectionPageLayout.Content>
        <Tabs defaultValue='assets'>
          <TabsList>
            <TabsTrigger value='asset-groups'>{t('Asset Groups')}</TabsTrigger>
            <TabsTrigger value='assets'>{t('Assets')}</TabsTrigger>
            <TabsTrigger value='face-verifications'>
              {t('Face Verifications')}
            </TabsTrigger>
          </TabsList>
          <TabsContent value='asset-groups' className='mt-4'>
            <AssetGroupsTab />
          </TabsContent>
          <TabsContent value='assets' className='mt-4'>
            <AssetsTab />
          </TabsContent>
          <TabsContent value='face-verifications' className='mt-4'>
            <FaceVerificationsTab />
          </TabsContent>
        </Tabs>
      </SectionPageLayout.Content>
    </SectionPageLayout>
  )
}
