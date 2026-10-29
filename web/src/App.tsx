import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Alert, AppBar, Box, Button, Checkbox, Chip, CircularProgress, Dialog, DialogActions,
  DialogContent, DialogTitle, Divider, Drawer, FormControl, IconButton, InputAdornment,
  InputLabel, MenuItem, Paper, Select, Snackbar, Stack, Switch, Table, TableBody,
  TableCell, TableContainer, TableHead, TableRow, TextField, Toolbar, Tooltip,
  Typography, useMediaQuery, useTheme,
} from '@mui/material'
import AddRounded from '@mui/icons-material/AddRounded'
import AutorenewRounded from '@mui/icons-material/AutorenewRounded'
import CheckRounded from '@mui/icons-material/CheckRounded'
import ContentCopyRounded from '@mui/icons-material/ContentCopyRounded'
import DnsRounded from '@mui/icons-material/DnsRounded'
import DeleteOutlineRounded from '@mui/icons-material/DeleteOutlineRounded'
import EditOutlined from '@mui/icons-material/EditOutlined'
import HubOutlined from '@mui/icons-material/HubOutlined'
import LinkRounded from '@mui/icons-material/LinkRounded'
import MoreHorizRounded from '@mui/icons-material/MoreHorizRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import VisibilityRounded from '@mui/icons-material/VisibilityRounded'
import VisibilityOffRounded from '@mui/icons-material/VisibilityOffRounded'
import WifiTetheringRounded from '@mui/icons-material/WifiTetheringRounded'
import LogoutRounded from '@mui/icons-material/LogoutRounded'

type SubStatus = 'pending' | 'healthy' | 'degraded' | 'failed' | 'disabled' | string
type Subscription = { id: string; name: string; enabled: boolean; refreshIntervalSeconds: number; nodeCount: number; lastAttemptAt?: string | null; lastSuccessAt?: string | null; status: SubStatus; errorCode?: string | null }
type NodeItem = { id: string; name: string; type: string; sourceId: string; sourceName: string; enabled: boolean }
type OutputLink = { id: string; name: string; url: string; enabled: boolean; createdAt: string }
type GroupRule = { type: 'geosite' | 'domain-suffix'; value: string }
type RouteGroup = { id: string; name: string; enabled: boolean; rules: GroupRule[]; nodeMode: 'all_enabled' | 'selected'; nodeIds: string[]; strategy: 'url-test' | string; testUrl: string; intervalSeconds: number; timeoutMs: number; expectedStatus: string; lazy: boolean; builtIn: boolean }
type GroupInput = Omit<RouteGroup, 'id' | 'builtIn'>
type DirectRoutingSettings = { chinaDomainDirect: boolean; chinaIPDirect: boolean; domains: string[] }
type OutputState = { status: string; nodeCount: number; generatedAt?: string | null; revision?: number; url?: string; links?: OutputLink[] }
type Page = 'subscriptions' | 'nodes' | 'output' | 'groups'
type Notice = { message: string; severity: 'success' | 'error' | 'info' }
const API = '/api/v1'
const serviceGroupIds = new Set(['chatgpt', 'claude', 'docker', 'google-us', 'github', 'reddit'])
const regionGroupIds = new Set(['europe-us', 'asia'])
const manualMembershipGroupIds = new Set(['google-us', 'europe-us', 'asia'])

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API}${path}`, { ...init, headers: { ...(init?.body ? { 'Content-Type': 'application/json' } : {}), ...init?.headers } })
  if (response.status === 204) return undefined as T
  const data = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(data?.error?.message || data?.error?.code || `HTTP ${response.status}`)
  return data as T
}
async function fetchAllNodes() {
  const limit = 200
  const first = await api<{ items: NodeItem[]; total: number }>(`/nodes?limit=${limit}&offset=0`)
  const items = [...first.items]
  for (let offset = items.length; offset < first.total; offset += limit) {
    const page = await api<{ items: NodeItem[]; total: number }>(`/nodes?limit=${limit}&offset=${offset}`)
    items.push(...page.items)
    if (!page.items.length) break
  }
  return { items, total: first.total }
}
const fmtDate = (value?: string | null) => value ? new Intl.DateTimeFormat('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }).format(new Date(value)) : '—'
const statusText: Record<string, string> = { pending: '更新中', healthy: '正常', degraded: '部分异常', failed: '更新失败', disabled: '已暂停' }
const subscriptionErrorText: Record<string, string> = { unsupported_format: '不支持的订阅格式', invalid_subscription: '订阅内容无效' }

function Status({ status }: { status: SubStatus }) {
  const good = status === 'healthy'
  const pending = status === 'pending'
  const bad = status === 'failed' || status === 'degraded'
  return <span className={`status ${good ? 'good' : bad ? 'bad' : pending ? 'pending' : 'muted'}`}><i />{statusText[status] || status}</span>
}

export default function App() {
  const theme = useTheme()
  const mobile = useMediaQuery(theme.breakpoints.down('md'))
  const [page, setPage] = useState<Page>('subscriptions')
  const [subs, setSubs] = useState<Subscription[]>([])
  const [nodes, setNodes] = useState<NodeItem[]>([])
  const [nodeTotal, setNodeTotal] = useState(0)
  const [groups, setGroups] = useState<RouteGroup[]>([])
  const [directSettings, setDirectSettings] = useState<DirectRoutingSettings | null>(null)
  const [directDomainsText, setDirectDomainsText] = useState('')
  const [groupDialog, setGroupDialog] = useState<'create' | 'edit' | 'delete' | null>(null)
  const [activeGroup, setActiveGroup] = useState<RouteGroup | null>(null)
  const [output, setOutput] = useState<OutputState | null>(null)
  const [loading, setLoading] = useState(false)
  const [nodeQuery, setNodeQuery] = useState('')
  const [nodeFilter, setNodeFilter] = useState('')
  const [dialog, setDialog] = useState<'create' | 'edit' | 'delete' | null>(null)
  const [activeSub, setActiveSub] = useState<Subscription | null>(null)
  const [busy, setBusy] = useState<string | null>(null)
  const [notice, setNotice] = useState<Notice | null>(null)
  const [linkDialog, setLinkDialog] = useState<'create' | 'rename' | null>(null)
  const [activeLink, setActiveLink] = useState<OutputLink | null>(null)
  const [authReady, setAuthReady] = useState(false)
  const [authError, setAuthError] = useState('')

  useEffect(() => {
    fetch('/auth/me').then(async response => {
      if (response.status === 401) { window.location.replace('/auth/login'); return }
      if (!response.ok) throw new Error('登录状态无法验证')
      setAuthReady(true)
    }).catch(error => setAuthError(error instanceof Error ? error.message : '登录状态无法验证'))
  }, [])

  const load = useCallback(async (target: Page = page) => {
    setLoading(true)
    try {
      if (target === 'subscriptions') {
        const data = await api<{ items: Subscription[]; total: number }>('/subscriptions')
        setSubs(data.items)
      } else if (target === 'nodes') {
        const params = new URLSearchParams({ limit: '200', offset: '0' })
        if (nodeQuery.trim()) params.set('q', nodeQuery.trim())
        if (nodeFilter) params.set('subscriptionId', nodeFilter)
        const data = await api<{ items: NodeItem[]; total: number }>(`/nodes?${params}`)
        setNodes(data.items); setNodeTotal(data.total)
        const sources = await api<{ items: Subscription[]; total: number }>('/subscriptions')
        setSubs(sources.items)
      } else if (target === 'groups') {
        const [groupData, nodeData, directData] = await Promise.all([
          api<{ items: RouteGroup[]; total: number }>('/groups'), fetchAllNodes(),
          api<{ settings: DirectRoutingSettings }>('/direct-routing'),
        ])
        setGroups(groupData.items)
        setNodes(nodeData.items); setNodeTotal(nodeData.total)
        setDirectSettings(directData.settings)
        setDirectDomainsText(directData.settings.domains.join('\n'))
      } else {
        const data = await api<OutputState>('/output')
        setOutput(data)
      }
    } catch (error) {
      setNotice({ message: error instanceof Error ? error.message : '请求失败', severity: 'error' })
    } finally { setLoading(false) }
  }, [page, nodeQuery, nodeFilter])

  useEffect(() => { if (authReady) void load(page) }, [page, load, authReady])

  const logout = async () => {
    await fetch('/auth/logout', { method: 'POST' })
    window.location.replace('/auth/login')
  }

  const counters = useMemo(() => ({ total: subs.length, enabled: subs.filter(s => s.enabled).length, nodes: subs.reduce((n, s) => n + (s.nodeCount || 0), 0), issues: subs.filter(s => s.status === 'failed' || s.status === 'degraded').length }), [subs])

  const run = async (key: string, action: () => Promise<unknown>, success: string, after?: () => Promise<void> | void) => {
    setBusy(key)
    try { await action(); setNotice({ message: success, severity: 'success' }); await after?.() }
    catch (error) { setNotice({ message: error instanceof Error ? error.message : '请求失败', severity: 'error' }) }
    finally { setBusy(null) }
  }

  const createSub = async (values: { name: string; url?: string; refreshIntervalSeconds?: number }) => {
    await run('save', () => api('/subscriptions', { method: 'POST', body: JSON.stringify(values) }), '订阅已添加', async () => { setDialog(null); await load('subscriptions') })
  }
  const editSub = async (values: { name: string; url?: string; refreshIntervalSeconds?: number; enabled?: boolean }) => {
    if (!activeSub) return
    await run('save', () => api(`/subscriptions/${encodeURIComponent(activeSub.id)}`, { method: 'PATCH', body: JSON.stringify(values) }), '订阅已保存', async () => { setDialog(null); await load('subscriptions') })
  }
  const refreshSub = (sub: Subscription) => run(`refresh:${sub.id}`, () => api(`/subscriptions/${encodeURIComponent(sub.id)}/refresh`, { method: 'POST' }), '已加入刷新队列', async () => { await load('subscriptions') })
  const toggleSub = (sub: Subscription) => run(`toggle:${sub.id}`, () => api(`/subscriptions/${encodeURIComponent(sub.id)}`, { method: 'PATCH', body: JSON.stringify({ enabled: !sub.enabled }) }), sub.enabled ? '订阅已暂停' : '订阅已启用', async () => { await load('subscriptions') })
  const toggleNode = (node: NodeItem) => run(`node-toggle:${node.id}`, () => api(`/nodes/${encodeURIComponent(node.id)}`, { method: 'PATCH', body: JSON.stringify({ enabled: !node.enabled }) }), node.enabled ? '节点已停用' : '节点已启用', async () => { await load('nodes') })
  const deleteSub = () => activeSub && run('delete', () => api(`/subscriptions/${encodeURIComponent(activeSub.id)}`, { method: 'DELETE' }), '订阅已删除', async () => { setDialog(null); await load('subscriptions') })
  const updateGroupInState = (group: RouteGroup) => setGroups(current => current.map(item => item.id === group.id ? group : item))
  const createGroup = (values: GroupInput) => run('group-create', async () => {
    const result = await api<{ group: RouteGroup }>('/groups', { method: 'POST', body: JSON.stringify(values) })
    setGroups(current => [...current, result.group])
  }, '策略组已创建', () => setGroupDialog(null))
  const editGroup = (values: GroupInput) => activeGroup && run('group-edit', async () => {
    const result = await api<{ group: RouteGroup }>(`/groups/${encodeURIComponent(activeGroup.id)}`, { method: 'PATCH', body: JSON.stringify(values) })
    updateGroupInState(result.group)
  }, '策略组已保存', () => { setGroupDialog(null); setActiveGroup(null) })
  const toggleGroup = (group: RouteGroup) => run(`group-toggle:${group.id}`, async () => {
    const result = await api<{ group: RouteGroup }>(`/groups/${encodeURIComponent(group.id)}`, { method: 'PATCH', body: JSON.stringify({ enabled: !group.enabled }) })
    updateGroupInState(result.group)
  }, group.enabled ? '策略组已停用' : '策略组已启用')
  const deleteGroup = () => activeGroup && run('group-delete', async () => {
    await api(`/groups/${encodeURIComponent(activeGroup.id)}`, { method: 'DELETE' })
    setGroups(current => current.filter(group => group.id !== activeGroup.id))
  }, '策略组已删除', () => { setGroupDialog(null); setActiveGroup(null) })
  const updateDirectSettings = (patch: Partial<DirectRoutingSettings>, success = '直连规则已保存') => run('direct-routing', async () => {
    const result = await api<{ settings: DirectRoutingSettings }>('/direct-routing', { method: 'PATCH', body: JSON.stringify(patch) })
    setDirectSettings(result.settings)
    setDirectDomainsText(result.settings.domains.join('\n'))
  }, success)
  const saveDirectDomains = () => updateDirectSettings({ domains: directDomainsText.split(/\r?\n/).map(domain => domain.trim()).filter(Boolean) })
  const copyOutputLink = async (link: OutputLink) => {
    try { await navigator.clipboard.writeText(link.url); setNotice({ message: '链接已复制', severity: 'success' }) }
    catch { setNotice({ message: '复制失败', severity: 'error' }) }
  }
  const saveOutputLink = (link: OutputLink) => setOutput(current => current ? { ...current, links: (current.links || []).some(item => item.id === link.id) ? current.links!.map(item => item.id === link.id ? link : item) : [...(current.links || []), link] } : current)
  const createOutputLink = (name: string) => run('link-create', async () => {
    const result = await api<{ link: OutputLink }>('/output/links', { method: 'POST', body: JSON.stringify({ name }) })
    saveOutputLink(result.link)
  }, '链接已创建', () => { setLinkDialog(null) })
  const renameOutputLink = (name: string) => activeLink && run('link-rename', async () => {
    const result = await api<{ link: OutputLink }>(`/output/links/${encodeURIComponent(activeLink.id)}`, { method: 'PATCH', body: JSON.stringify({ name }) })
    saveOutputLink(result.link)
  }, '名称已更新', () => { setLinkDialog(null); setActiveLink(null) })
  const toggleOutputLink = (link: OutputLink) => run(`link-toggle:${link.id}`, async () => {
    const result = await api<{ link: OutputLink }>(`/output/links/${encodeURIComponent(link.id)}`, { method: 'PATCH', body: JSON.stringify({ enabled: !link.enabled }) })
    saveOutputLink(result.link)
  }, link.enabled ? '链接已暂停' : '链接已启用')

  const nav = <Stack spacing={0.5} sx={{ p: 2, pt: mobile ? 0 : 2 }}>
    {[['subscriptions', '订阅', <AutorenewRounded key="s"/>], ['nodes', '节点', <HubOutlined key="n"/>], ['output', '统一出口', <LinkRounded key="o"/>], ['groups', '策略组', <DnsRounded key="g"/>]] .map(([key, title, icon]) => <Button key={key as string} startIcon={icon} onClick={() => setPage(key as Page)} className={`nav-item ${page === key ? 'selected' : ''}`} fullWidth>{title}</Button>)}
  </Stack>

  if (authError) return <Box sx={{ minHeight: '100vh', display: 'grid', placeItems: 'center' }}><Stack alignItems="center" spacing={2}><Typography>{authError}</Typography><Button variant="contained" onClick={() => window.location.replace('/auth/login')}>重新登录</Button></Stack></Box>
  if (!authReady) return <Box sx={{ minHeight: '100vh', display: 'grid', placeItems: 'center' }}><CircularProgress size={24}/></Box>

  return <Box className="app-shell">
    {!mobile && <Drawer variant="permanent" className="side-drawer" PaperProps={{ className: 'drawer-paper' }}><Box className="brand"><span className="brand-mark"><WifiTetheringRounded/></span><Typography variant="subtitle1" fontWeight={750}>CLASH CLOUD</Typography></Box><Divider sx={{ borderColor: 'rgba(255,255,255,.12)' }}/>{nav}</Drawer>}
    <Box component="main" className="main-area">
      {mobile && <AppBar position="sticky" elevation={0} className="mobile-bar"><Toolbar><span className="brand-mark"><WifiTetheringRounded/></span><Typography fontWeight={750}>CLASH CLOUD</Typography></Toolbar>{nav}</AppBar>}
      <Box className="content">
        <Box className="page-heading"><Box><Typography variant="h4">{{ subscriptions: '订阅', nodes: '节点', output: '统一出口', groups: '策略组' }[page]}</Typography></Box><Stack direction="row" spacing={1} alignItems="center">{page === 'subscriptions' && <Button variant="contained" startIcon={<AddRounded/>} onClick={() => { setActiveSub(null); setDialog('create') }}>添加订阅</Button>}{page === 'groups' && <Button variant="contained" startIcon={<AddRounded/>} onClick={() => { setActiveGroup(null); setGroupDialog('create') }}>新建组</Button>}{page === 'nodes' && <Tooltip title="刷新"><IconButton onClick={() => void load('nodes')}><AutorenewRounded/></IconButton></Tooltip>}<Tooltip title="退出登录"><IconButton onClick={() => void logout()} aria-label="退出登录"><LogoutRounded/></IconButton></Tooltip></Stack></Box>

        {page === 'subscriptions' && <>
          <Box className="stats-row">
            <Stat value={counters.total} label="订阅"/><Stat value={counters.enabled} label="启用"/><Stat value={counters.nodes} label="节点"/><Stat value={counters.issues} label="异常" warning={counters.issues > 0}/>
          </Box>
          <Paper className="table-paper" variant="outlined">
            <TableContainer><Table aria-label="订阅列表">
              <TableHead><TableRow><TableCell>名称</TableCell><TableCell align="right">节点</TableCell><TableCell>上次刷新</TableCell><TableCell>状态</TableCell><TableCell align="right">操作</TableCell></TableRow></TableHead>
              <TableBody>
                {loading && !subs.length ? <TableRow><TableCell colSpan={5}><LoadingRows/></TableCell></TableRow> : subs.map(sub => <TableRow key={sub.id} hover>
                  <TableCell><Typography fontWeight={650}>{sub.name}</Typography>{sub.errorCode && <Typography className="error-code">{subscriptionErrorText[sub.errorCode] || '刷新失败'}</Typography>}</TableCell>
                  <TableCell align="right" className="numeric">{sub.nodeCount}</TableCell><TableCell className="date-cell">{fmtDate(sub.lastSuccessAt || sub.lastAttemptAt)}</TableCell><TableCell><Status status={sub.enabled ? sub.status : 'disabled'}/></TableCell>
                  <TableCell align="right"><Stack direction="row" spacing={0.25} alignItems="center" justifyContent="flex-end">
                    <Tooltip title="刷新"><span><IconButton size="small" aria-label={`刷新 ${sub.name}`} disabled={busy === `refresh:${sub.id}`} onClick={() => void refreshSub(sub)}>{busy === `refresh:${sub.id}` ? <CircularProgress size={18}/> : <AutorenewRounded fontSize="small"/>}</IconButton></span></Tooltip>
                    <Tooltip title={sub.enabled ? '暂停' : '启用'}><span><Switch size="small" checked={sub.enabled} disabled={busy === `toggle:${sub.id}`} onChange={() => void toggleSub(sub)} inputProps={{ 'aria-label': `${sub.enabled ? '暂停' : '启用'} ${sub.name}` }}/></span></Tooltip>
                    <Tooltip title="编辑"><IconButton size="small" aria-label={`编辑 ${sub.name}`} onClick={() => { setActiveSub(sub); setDialog('edit') }}><EditOutlined fontSize="small"/></IconButton></Tooltip>
                    <Tooltip title="删除"><IconButton size="small" aria-label={`删除 ${sub.name}`} onClick={() => { setActiveSub(sub); setDialog('delete') }}><DeleteOutlineRounded fontSize="small"/></IconButton></Tooltip>
                  </Stack></TableCell>
                </TableRow>)}
                {!loading && !subs.length && <TableRow><TableCell colSpan={5}><Empty title="还没有订阅" action={<Button variant="contained" startIcon={<AddRounded/>} onClick={() => setDialog('create')}>添加订阅</Button>}/></TableCell></TableRow>}
              </TableBody>
            </Table></TableContainer>
          </Paper>
          <OutputStrip output={output} onLoad={() => void load('output')} onManage={() => setPage('output')} />
        </>}

        {page === 'nodes' && <>
          <Box className="filter-row"><TextField size="small" placeholder="搜索节点" value={nodeQuery} onChange={e => setNodeQuery(e.target.value)} onKeyDown={e => { if (e.key === 'Enter') void load('nodes') }} InputProps={{ startAdornment: <InputAdornment position="start"><SearchRounded fontSize="small"/></InputAdornment> }} />
            <FormControl size="small" className="filter-select"><InputLabel id="source-filter">全部订阅</InputLabel><Select labelId="source-filter" label="全部订阅" value={nodeFilter} onChange={e => setNodeFilter(e.target.value)}><MenuItem value="">全部订阅</MenuItem>{subs.map(s => <MenuItem key={s.id} value={s.id}>{s.name}</MenuItem>)}</Select></FormControl>
            <Typography className="result-count">{nodeTotal} 个节点</Typography>
          </Box>
          <Paper className="table-paper" variant="outlined"><TableContainer><Table aria-label="节点列表"><TableHead><TableRow><TableCell>节点</TableCell><TableCell>来源</TableCell><TableCell>协议</TableCell><TableCell>统一出口</TableCell></TableRow></TableHead><TableBody>
            {loading && !nodes.length ? <TableRow><TableCell colSpan={4}><LoadingRows/></TableCell></TableRow> : nodes.map(node => <TableRow key={node.id} hover>
              <TableCell><Typography fontWeight={650}>{node.name}</Typography></TableCell><TableCell>{node.sourceName}</TableCell><TableCell><Chip size="small" label={node.type}/></TableCell><TableCell><Tooltip title={node.enabled ? '在统一出口停用' : '加入统一出口'}><span><Switch size="small" checked={node.enabled} disabled={busy === `node-toggle:${node.id}`} onChange={() => void toggleNode(node)} inputProps={{ 'aria-label': `${node.enabled ? '停用' : '启用'} ${node.name}` }}/></span></Tooltip></TableCell>
            </TableRow>)}
            {!loading && !nodes.length && <TableRow><TableCell colSpan={4}><Empty title="没有匹配的节点"/></TableCell></TableRow>}
          </TableBody></Table></TableContainer></Paper>
        </>}

        {page === 'groups' && <Stack spacing={2}>
          {directSettings && <DirectRoutingPanel settings={directSettings} domainsText={directDomainsText} busy={busy === 'direct-routing'} onDomainsChange={setDirectDomainsText} onToggleDomain={() => void updateDirectSettings({ chinaDomainDirect: !directSettings.chinaDomainDirect })} onToggleIP={() => void updateDirectSettings({ chinaIPDirect: !directSettings.chinaIPDirect })} onSaveDomains={saveDirectDomains}/>}
          <GroupTable groups={groups} nodes={nodes} loading={loading} busy={busy} onToggle={toggleGroup} onEdit={group => { setActiveGroup(group); setGroupDialog('edit') }} onDelete={group => { setActiveGroup(group); setGroupDialog('delete') }}/>
        </Stack>}
        {page === 'output' && <OutputPanel output={output} loading={loading} onRefresh={() => void load('output')} onCreate={() => { setActiveLink(null); setLinkDialog('create') }} onCopy={link => void copyOutputLink(link)} onRename={link => { setActiveLink(link); setLinkDialog('rename') }} onToggle={link => void toggleOutputLink(link)} busy={busy}/>}
      </Box>
    </Box>
    <GroupDialog open={groupDialog === 'create' || groupDialog === 'edit'} mode={groupDialog === 'edit' ? 'edit' : 'create'} initial={activeGroup} nodes={nodes} busy={busy === 'group-create' || busy === 'group-edit'} onClose={() => { setGroupDialog(null); setActiveGroup(null) }} onSave={groupDialog === 'edit' ? editGroup : createGroup}/>
    <Dialog open={groupDialog === 'delete'} onClose={() => { setGroupDialog(null); setActiveGroup(null) }} maxWidth="xs" fullWidth><DialogTitle>删除策略组</DialogTitle><DialogContent><Typography>确定删除「{activeGroup?.name}」？</Typography></DialogContent><DialogActions><Button onClick={() => { setGroupDialog(null); setActiveGroup(null) }}>取消</Button><Button color="error" variant="contained" disabled={busy === 'group-delete'} onClick={() => void deleteGroup()}>{busy === 'group-delete' ? '删除中…' : '删除'}</Button></DialogActions></Dialog>
    <SubscriptionDialog open={dialog === 'create' || dialog === 'edit'} mode={dialog === 'edit' ? 'edit' : 'create'} sub={activeSub} busy={busy === 'save'} onClose={() => setDialog(null)} onSave={dialog === 'edit' ? editSub : createSub}/>
    <OutputLinkDialog open={!!linkDialog} mode={linkDialog === 'rename' ? 'rename' : 'create'} initialName={activeLink?.name || ''} busy={busy === 'link-create' || busy === 'link-rename'} onClose={() => { setLinkDialog(null); setActiveLink(null) }} onSave={linkDialog === 'rename' ? renameOutputLink : createOutputLink}/>
    <Dialog open={dialog === 'delete'} onClose={() => setDialog(null)} maxWidth="xs" fullWidth><DialogTitle>删除订阅</DialogTitle><DialogContent><Typography>确定删除「{activeSub?.name}」？</Typography></DialogContent><DialogActions><Button onClick={() => setDialog(null)}>取消</Button><Button color="error" variant="contained" disabled={busy === 'delete'} onClick={() => void deleteSub()}>{busy === 'delete' ? '删除中…' : '删除'}</Button></DialogActions></Dialog>
    <Snackbar open={!!notice} autoHideDuration={3500} onClose={() => setNotice(null)} anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}><Alert severity={notice?.severity || 'info'} variant="filled" onClose={() => setNotice(null)}>{notice?.message}</Alert></Snackbar>
  </Box>
}

function Stat({ value, label, warning = false }: { value: number; label: string; warning?: boolean }) { return <Box className="stat"><Typography className={warning ? 'stat-value warning' : 'stat-value'}>{value}</Typography><Typography className="stat-label">{label}</Typography></Box> }
function LoadingRows() { return <Stack direction="row" alignItems="center" justifyContent="center" spacing={1} sx={{ py: 4, color: 'text.secondary' }}><CircularProgress size={18}/><Typography>加载中</Typography></Stack> }
function Empty({ title, action }: { title: string; action?: React.ReactNode }) { return <Box className="empty"><Typography>{title}</Typography>{action}</Box> }

function SubscriptionDialog({ open, mode, sub, busy, onClose, onSave }: { open: boolean; mode: 'create' | 'edit'; sub: Subscription | null; busy: boolean; onClose: () => void; onSave: (v: { name: string; url?: string; refreshIntervalSeconds?: number; enabled?: boolean }) => void }) {
  const [name, setName] = useState('')
  const [url, setUrl] = useState('')
  const [interval, setInterval] = useState('')
  const [showUrl, setShowUrl] = useState(false)
  const [enabled, setEnabled] = useState(true)
  const [error, setError] = useState('')
  useEffect(() => { if (open) { setName(sub?.name || ''); setUrl(''); setInterval(sub?.refreshIntervalSeconds ? String(sub.refreshIntervalSeconds) : ''); setError(''); setShowUrl(false); setEnabled(sub?.enabled ?? true) } }, [open, sub])
  const submit = () => {
    if (!name.trim()) { setError('请填写订阅名称'); return }
    if (mode === 'create' && !url.trim()) { setError('请填写订阅地址'); return }
    const seconds = Number(interval)
    onSave({ name: name.trim(), ...(url.trim() ? { url: url.trim() } : {}), ...(Number.isFinite(seconds) && seconds > 0 ? { refreshIntervalSeconds: seconds } : {}), ...(mode === 'edit' ? { enabled } : {}) })
  }
  return <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth><DialogTitle>{mode === 'create' ? '添加订阅' : '编辑订阅'}</DialogTitle><DialogContent className="form-content">
    <TextField autoFocus fullWidth label="订阅名称" value={name} onChange={e => { setName(e.target.value); setError('') }} error={!!error && !name.trim()} />
    <TextField fullWidth label={mode === 'create' ? '订阅地址' : '更换订阅地址'} type={showUrl ? 'text' : 'password'} required={mode === 'create'} value={url} onChange={e => { setUrl(e.target.value); setError('') }} error={!!error && mode === 'create' && !url.trim()} helperText={error && mode === 'create' && !url.trim() ? error : undefined} InputProps={{ endAdornment: <InputAdornment position="end"><IconButton aria-label={showUrl ? '隐藏订阅地址' : '显示订阅地址'} onClick={() => setShowUrl(v => !v)}>{showUrl ? <VisibilityOffRounded/> : <VisibilityRounded/>}</IconButton></InputAdornment> }}/>
    <TextField fullWidth label="自动刷新间隔（秒）" type="number" value={interval} onChange={e => setInterval(e.target.value)} inputProps={{ min: 60 }}/>
    {mode === 'edit' && sub && <Stack direction="row" alignItems="center" justifyContent="space-between"><Typography>启用订阅</Typography><Switch checked={enabled} onChange={(_, checked) => setEnabled(checked)}/></Stack>}
  </DialogContent><DialogActions><Button onClick={onClose}>取消</Button><Button variant="contained" disabled={busy} onClick={submit}>{busy ? '保存中…' : mode === 'create' ? '添加订阅' : '保存'}</Button></DialogActions></Dialog>
}

function emptyGroup(): GroupInput {
  return { name: '', enabled: true, rules: [], nodeMode: 'all_enabled', nodeIds: [], strategy: 'url-test', testUrl: 'https://www.gstatic.com/generate_204', intervalSeconds: 180, timeoutMs: 5000, expectedStatus: '204', lazy: false }
}
function DirectRoutingPanel({ settings, domainsText, busy, onDomainsChange, onToggleDomain, onToggleIP, onSaveDomains }: { settings: DirectRoutingSettings; domainsText: string; busy: boolean; onDomainsChange: (value: string) => void; onToggleDomain: () => void; onToggleIP: () => void; onSaveDomains: () => void }) {
  return <Paper variant="outlined" sx={{ p: 2.25, borderColor: 'divider' }}>
    <Stack spacing={2}>
      <Typography fontWeight={700}>直连规则</Typography>
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5}>
        <Stack direction="row" alignItems="center" justifyContent="space-between" className="group-enable" sx={{ flex: 1 }}><Typography>大陆域名直连</Typography><Switch size="small" checked={settings.chinaDomainDirect} disabled={busy} onChange={onToggleDomain}/></Stack>
        <Stack direction="row" alignItems="center" justifyContent="space-between" className="group-enable" sx={{ flex: 1 }}><Typography>大陆 IP 直连</Typography><Switch size="small" checked={settings.chinaIPDirect} disabled={busy} onChange={onToggleIP}/></Stack>
      </Stack>
      <Stack direction={{ xs: 'column', sm: 'row' }} spacing={1.5} alignItems={{ sm: 'flex-start' }}>
        <TextField fullWidth multiline minRows={2} label="指定域名直连" placeholder="example.cn" value={domainsText} onChange={event => onDomainsChange(event.target.value)}/>
        <Button variant="contained" disabled={busy} onClick={onSaveDomains} sx={{ minWidth: 140 }}>保存直连规则</Button>
      </Stack>
    </Stack>
  </Paper>
}

function selectedEnabledCount(group: RouteGroup, nodes: NodeItem[]) {
  if (group.nodeMode === 'all_enabled') return nodes.filter(node => node.enabled).length
  const ids = new Set(group.nodeIds || [])
  return nodes.filter(node => ids.has(node.id) && node.enabled).length
}
function GroupTable({ groups, nodes, loading, busy, onToggle, onEdit, onDelete }: { groups: RouteGroup[]; nodes: NodeItem[]; loading: boolean; busy: string | null; onToggle: (group: RouteGroup) => void; onEdit: (group: RouteGroup) => void; onDelete: (group: RouteGroup) => void }) {
  return <Paper className="table-paper groups-table" variant="outlined"><TableContainer><Table aria-label="策略组列表"><TableHead><TableRow><TableCell>名称</TableCell><TableCell>规则</TableCell><TableCell>节点</TableCell><TableCell>策略</TableCell><TableCell>状态</TableCell><TableCell align="right">操作</TableCell></TableRow></TableHead><TableBody>
    {loading && !groups.length ? <TableRow><TableCell colSpan={6}><LoadingRows/></TableCell></TableRow> : groups.map(group => {
      const memberCount = selectedEnabledCount(group, nodes)
      const emptySelection = group.nodeMode === 'selected' && memberCount === 0
      const noDomains = !group.rules?.length
      return <TableRow key={group.id} hover>
        <TableCell><Stack direction="row" spacing={1} alignItems="center"><Typography fontWeight={650}>{group.name}</Typography>{group.id === 'general' && <Chip size="small" label="默认"/>}</Stack><Typography className="group-subtitle">{group.id === 'general' ? '默认组' : serviceGroupIds.has(group.id) ? '服务组' : regionGroupIds.has(group.id) ? '地区组' : '自定义组'}</Typography></TableCell>
        <TableCell>{noDomains ? '—' : `${group.rules?.length || 0} 条`}</TableCell>
        <TableCell>{emptySelection ? <Typography color="warning.main">未配置节点</Typography> : group.nodeMode === 'all_enabled' ? '全部启用节点' : `${memberCount} 个节点`}</TableCell>
        <TableCell>{group.strategy === 'url-test' ? `URL 测试 · ${group.intervalSeconds}s` : group.strategy}</TableCell>
        <TableCell><Stack direction="row" alignItems="center" spacing={0.75}><Switch size="small" checked={group.enabled} disabled={group.id === 'general' || busy === `group-toggle:${group.id}` || (!group.enabled && emptySelection)} onChange={() => onToggle(group)} inputProps={{ 'aria-label': `${group.enabled ? '停用' : '启用'} ${group.name}` }}/><Typography color={emptySelection ? 'warning.main' : undefined}>{emptySelection ? '未配置节点' : group.enabled ? '启用' : '已停用'}</Typography></Stack></TableCell>
        <TableCell align="right"><Stack direction="row" spacing={0.25} justifyContent="flex-end"><Tooltip title="编辑"><IconButton size="small" aria-label={`编辑 ${group.name}`} onClick={() => onEdit(group)}><EditOutlined fontSize="small"/></IconButton></Tooltip>{!group.builtIn && <Tooltip title="删除"><IconButton size="small" aria-label={`删除 ${group.name}`} onClick={() => onDelete(group)}><DeleteOutlineRounded fontSize="small"/></IconButton></Tooltip>}</Stack></TableCell>
      </TableRow>
    })}
    {!loading && !groups.length && <TableRow><TableCell colSpan={6}><Empty title="还没有策略组"/></TableCell></TableRow>}
  </TableBody></Table></TableContainer></Paper>
}
function GroupDialog({ open, mode, initial, nodes, busy, onClose, onSave }: { open: boolean; mode: 'create' | 'edit'; initial: RouteGroup | null; nodes: NodeItem[]; busy: boolean; onClose: () => void; onSave: (values: GroupInput) => void }) {
  const [form, setForm] = useState<GroupInput>(emptyGroup())
  const [error, setError] = useState('')
  useEffect(() => { if (open) { const { id: _id, builtIn: _builtIn, ...editable } = initial || { id: '', builtIn: false, ...emptyGroup() }; setForm({ ...emptyGroup(), ...editable, rules: [...(editable.rules || [])], nodeIds: [...(editable.nodeIds || [])] }); setError('') } }, [open, initial])
  const set = <K extends keyof GroupInput,>(key: K, value: GroupInput[K]) => setForm(current => ({ ...current, [key]: value }))
  const regionManual = manualMembershipGroupIds.has(initial?.id || '')
  const keepGeneral = initial?.id === 'general'
  const rulesEditable = !keepGeneral
  const selectedNodes = new Set(form.nodeIds)
  const submit = () => {
    if (!form.name.trim()) { setError('请填写名称'); return }
    if (!form.testUrl.trim()) { setError('请填写测试 URL'); return }
    onSave({ ...form, name: form.name.trim(), enabled: keepGeneral || form.enabled, rules: rulesEditable ? form.rules.filter(rule => rule.value.trim()).map(rule => ({ ...rule, value: rule.value.trim() })) : [], nodeMode: keepGeneral ? 'all_enabled' : regionManual ? 'selected' : form.nodeMode, nodeIds: keepGeneral || form.nodeMode === 'all_enabled' ? [] : form.nodeIds })
  }
  return <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth><DialogTitle>{mode === 'create' ? '新建策略组' : `编辑策略组 · ${initial?.name || ''}`}</DialogTitle><DialogContent className="group-dialog-content">
    <Box className="group-form-grid"><TextField label="名称" value={form.name} disabled={initial?.id === 'general'} onChange={event => { set('name', event.target.value); setError('') }} error={!!error && !form.name.trim()} inputProps={{ maxLength: 80 }}/>
      <Stack direction="row" alignItems="center" justifyContent="space-between" className="group-enable"><Typography>启用</Typography><Switch checked={keepGeneral || form.enabled} disabled={keepGeneral} onChange={(_, value) => set('enabled', value)}/></Stack>
      <FormControl><InputLabel id="node-mode-label">节点范围</InputLabel><Select labelId="node-mode-label" label="节点范围" value={keepGeneral ? 'all_enabled' : regionManual ? 'selected' : form.nodeMode} disabled={keepGeneral || regionManual} onChange={event => set('nodeMode', event.target.value as GroupInput['nodeMode'])}><MenuItem value="all_enabled">全部启用节点</MenuItem><MenuItem value="selected">手动选择</MenuItem></Select></FormControl>
      <FormControl><InputLabel id="strategy-label">策略</InputLabel><Select labelId="strategy-label" label="策略" value={form.strategy} onChange={event => set('strategy', event.target.value)}><MenuItem value="url-test">URL 测试</MenuItem></Select></FormControl>
    </Box>
    {rulesEditable && <Box className="group-section"><Stack direction="row" alignItems="center" justifyContent="space-between"><Typography fontWeight={650}>域名规则</Typography><Button size="small" startIcon={<AddRounded/>} onClick={() => set('rules', [...form.rules, { type: 'domain-suffix', value: '' }])}>添加规则</Button></Stack>
      {form.rules.map((rule, index) => <Stack key={`${index}:${rule.type}`} direction={{ xs: 'column', sm: 'row' }} spacing={1} className="group-rule-row"><FormControl size="small" className="rule-type"><Select value={rule.type} onChange={event => set('rules', form.rules.map((item, i) => i === index ? { ...item, type: event.target.value as GroupRule['type'] } : item))}><MenuItem value="domain-suffix">域名后缀</MenuItem><MenuItem value="geosite">Geosite</MenuItem></Select></FormControl><TextField size="small" fullWidth label={rule.type === 'geosite' ? 'Geosite' : '域名'} value={rule.value} onChange={event => set('rules', form.rules.map((item, i) => i === index ? { ...item, value: event.target.value } : item))}/><IconButton aria-label="删除规则" onClick={() => set('rules', form.rules.filter((_, i) => i !== index))}><DeleteOutlineRounded/></IconButton></Stack>)}
    </Box>}
    {form.nodeMode === 'selected' && <FormControl fullWidth><InputLabel id="node-select-label">指定节点</InputLabel><Select labelId="node-select-label" multiple label="指定节点" value={form.nodeIds} onChange={event => set('nodeIds', typeof event.target.value === 'string' ? event.target.value.split(',') : event.target.value)} renderValue={ids => (ids as string[]).map(id => nodes.find(node => node.id === id)?.name || id).join('、')}><MenuItem disabled value=""><em>选择节点</em></MenuItem>{nodes.map(node => <MenuItem key={node.id} value={node.id}><Checkbox checked={selectedNodes.has(node.id)}/><Typography>{node.name}{node.enabled ? '' : '（已停用）'}</Typography></MenuItem>)}</Select></FormControl>}
    <Typography fontWeight={650} className="group-section-title">测速</Typography><Box className="group-form-grid"><TextField label="测试 URL" value={form.testUrl} onChange={event => { set('testUrl', event.target.value); setError('') }} error={!!error && !form.testUrl.trim()}/><TextField label="间隔（秒）" type="number" value={form.intervalSeconds} onChange={event => set('intervalSeconds', Number(event.target.value))} inputProps={{ min: 1 }}/><TextField label="超时（毫秒）" type="number" value={form.timeoutMs} onChange={event => set('timeoutMs', Number(event.target.value))} inputProps={{ min: 0 }}/><TextField label="预期 HTTP 状态码" value={form.expectedStatus} onChange={event => set('expectedStatus', event.target.value)} inputProps={{ maxLength: 16 }}/><Stack direction="row" alignItems="center" justifyContent="space-between"><Typography>按需测速</Typography><Switch checked={form.lazy} onChange={(_, value) => set('lazy', value)}/></Stack></Box>
    {error && <Typography color="error" fontSize={14}>{error}</Typography>}
  </DialogContent><DialogActions><Button onClick={onClose}>取消</Button><Button variant="contained" disabled={busy} onClick={submit}>{busy ? '保存中…' : mode === 'create' ? '创建' : '保存'}</Button></DialogActions></Dialog>
}

function maskOutputUrl(url: string) { return url.replace(/(\/s\/)[^/?#]+/, '$1••••••••••') }
function OutputStrip({ output, onLoad, onManage }: { output: OutputState | null; onLoad: () => void; onManage: () => void }) {
  useEffect(() => { if (!output) onLoad() }, [output, onLoad])
  const count = output?.links?.filter(link => link.enabled).length ?? 0
  return <Box className="output-strip"><Stack direction="row" alignItems="center" spacing={1} className="output-label"><Status status={output?.status === 'healthy' ? 'healthy' : output?.nodeCount ? 'pending' : 'failed'}/><Typography fontWeight={650}>统一出口</Typography></Stack><Box className="url-preview">{count} 个可用链接</Box><Button size="small" onClick={onManage}>管理链接</Button></Box>
}
function OutputPanel({ output, loading, onRefresh, onCreate, onCopy, onRename, onToggle, busy }: { output: OutputState | null; loading: boolean; onRefresh: () => void; onCreate: () => void; onCopy: (link: OutputLink) => void; onRename: (link: OutputLink) => void; onToggle: (link: OutputLink) => void; busy: string | null }) {
  const links = output?.links || []
  return <Stack spacing={2}>
    <Paper variant="outlined" className="output-panel"><Stack direction="row" alignItems="center" justifyContent="space-between"><Box><Typography variant="h6" fontWeight={700}>统一出口</Typography><Box sx={{ mt: 1 }}><Status status={output?.status === 'healthy' ? 'healthy' : output?.nodeCount ? 'pending' : 'failed'}/></Box></Box><Stack direction="row" spacing={1}><Button onClick={onRefresh} startIcon={<AutorenewRounded/>}>刷新</Button><Button variant="contained" startIcon={<AddRounded/>} onClick={onCreate}>新建链接</Button></Stack></Stack>
      <Box className="output-meta"><Meta label="节点数" value={String(output?.nodeCount ?? 0)}/><Meta label="配置版本" value={output?.revision ? `#${output.revision}` : '—'}/><Meta label="生成时间" value={fmtDate(output?.generatedAt)}/></Box>
    </Paper>
    <Paper variant="outlined" className="table-paper"><TableContainer><Table aria-label="统一出口链接"><TableHead><TableRow><TableCell>名称</TableCell><TableCell>订阅地址</TableCell><TableCell>创建时间</TableCell><TableCell>状态</TableCell><TableCell align="right">操作</TableCell></TableRow></TableHead><TableBody>
      {loading && !links.length ? <TableRow><TableCell colSpan={5}><LoadingRows/></TableCell></TableRow> : links.map(link => <TableRow key={link.id} hover>
        <TableCell><Typography fontWeight={650}>{link.name}</Typography></TableCell><TableCell className="url-preview">{maskOutputUrl(link.url)}</TableCell><TableCell className="date-cell">{fmtDate(link.createdAt)}</TableCell><TableCell><Stack direction="row" alignItems="center" spacing={1}><Switch size="small" checked={link.enabled} disabled={busy === `link-toggle:${link.id}`} onChange={() => onToggle(link)} inputProps={{ 'aria-label': `${link.enabled ? '暂停' : '启用'} ${link.name}` }}/><Typography>{link.enabled ? '启用' : '已暂停'}</Typography></Stack></TableCell><TableCell align="right"><Stack direction="row" spacing={0.5} justifyContent="flex-end"><Tooltip title="复制链接"><IconButton size="small" aria-label={`复制 ${link.name}`} onClick={() => onCopy(link)}><ContentCopyRounded fontSize="small"/></IconButton></Tooltip><Tooltip title="重命名"><IconButton size="small" aria-label={`重命名 ${link.name}`} onClick={() => onRename(link)}><EditOutlined fontSize="small"/></IconButton></Tooltip></Stack></TableCell>
      </TableRow>)}
      {!loading && !links.length && <TableRow><TableCell colSpan={5}><Empty title="还没有订阅链接" action={<Button variant="contained" startIcon={<AddRounded/>} onClick={onCreate}>新建链接</Button>}/></TableCell></TableRow>}
    </TableBody></Table></TableContainer></Paper>
  </Stack>
}
function OutputLinkDialog({ open, mode, initialName, busy, onClose, onSave }: { open: boolean; mode: 'create' | 'rename'; initialName: string; busy: boolean; onClose: () => void; onSave: (name: string) => void }) {
  const [name, setName] = useState(initialName)
  const [error, setError] = useState('')
  useEffect(() => { if (open) { setName(initialName); setError('') } }, [open, initialName])
  const submit = () => {
    if (!name.trim()) { setError('请填写名称'); return }
    if (name.trim().length > 80) { setError('名称最长 80 个字符'); return }
    onSave(name.trim())
  }
  return <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth><DialogTitle>{mode === 'create' ? '新建订阅链接' : '重命名链接'}</DialogTitle><DialogContent className="form-content"><TextField autoFocus fullWidth label="链接名称" value={name} inputProps={{ maxLength: 80 }} error={!!error} helperText={error || undefined} onChange={event => { setName(event.target.value); setError('') }}/></DialogContent><DialogActions><Button onClick={onClose}>取消</Button><Button variant="contained" disabled={busy} onClick={submit}>{busy ? '保存中…' : mode === 'create' ? '创建' : '保存'}</Button></DialogActions></Dialog>
}
function Meta({ label, value }: { label: string; value: string }) { return <Box><Typography className="meta-label">{label}</Typography><Typography fontWeight={650}>{value}</Typography></Box> }
