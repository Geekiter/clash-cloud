import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  Alert, AppBar, Box, Button, Chip, CircularProgress, Dialog, DialogActions,
  DialogContent, DialogTitle, Divider, Drawer, FormControl, IconButton, InputAdornment,
  InputLabel, MenuItem, Paper, Select, Snackbar, Stack, Switch, Table, TableBody,
  TableCell, TableContainer, TableHead, TableRow, TextField, Toolbar, Tooltip,
  Typography, useMediaQuery, useTheme,
} from '@mui/material'
import AddRounded from '@mui/icons-material/AddRounded'
import AutorenewRounded from '@mui/icons-material/AutorenewRounded'
import CheckRounded from '@mui/icons-material/CheckRounded'
import ContentCopyRounded from '@mui/icons-material/ContentCopyRounded'
import DeleteOutlineRounded from '@mui/icons-material/DeleteOutlineRounded'
import EditOutlined from '@mui/icons-material/EditOutlined'
import HubOutlined from '@mui/icons-material/HubOutlined'
import LinkRounded from '@mui/icons-material/LinkRounded'
import MoreHorizRounded from '@mui/icons-material/MoreHorizRounded'
import NetworkCheckRounded from '@mui/icons-material/NetworkCheckRounded'
import SearchRounded from '@mui/icons-material/SearchRounded'
import VisibilityRounded from '@mui/icons-material/VisibilityRounded'
import VisibilityOffRounded from '@mui/icons-material/VisibilityOffRounded'
import WifiTetheringRounded from '@mui/icons-material/WifiTetheringRounded'
import LogoutRounded from '@mui/icons-material/LogoutRounded'

type SubStatus = 'pending' | 'healthy' | 'degraded' | 'failed' | 'disabled' | string
type Subscription = { id: string; name: string; enabled: boolean; refreshIntervalSeconds: number; nodeCount: number; lastAttemptAt?: string | null; lastSuccessAt?: string | null; status: SubStatus; errorCode?: string | null }
type NodeItem = { id: string; name: string; type: string; sourceId: string; sourceName: string; latencyMs?: number | null; available?: boolean | null; selected?: boolean }
type OutputLink = { id: string; name: string; url: string; enabled: boolean; createdAt: string }
type OutputState = { status: string; nodeCount: number; generatedAt?: string | null; revision?: number; url?: string; links?: OutputLink[]; selectedGroup?: string }
type Page = 'subscriptions' | 'nodes' | 'output'
type Notice = { message: string; severity: 'success' | 'error' | 'info' }
const API = '/api/v1'

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(`${API}${path}`, { ...init, headers: { ...(init?.body ? { 'Content-Type': 'application/json' } : {}), ...init?.headers } })
  if (response.status === 204) return undefined as T
  const data = await response.json().catch(() => ({}))
  if (!response.ok) throw new Error(data?.error?.message || data?.error?.code || `HTTP ${response.status}`)
  return data as T
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
  const [testBusy, setTestBusy] = useState<string | null>(null)
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
  const deleteSub = () => activeSub && run('delete', () => api(`/subscriptions/${encodeURIComponent(activeSub.id)}`, { method: 'DELETE' }), '订阅已删除', async () => { setDialog(null); await load('subscriptions') })
  const testNode = async (node: NodeItem) => {
    setTestBusy(node.id)
    try {
      const result = await api<{ delayMs: number }>(`/nodes/${encodeURIComponent(node.id)}/delay-test`, { method: 'POST', body: JSON.stringify({ url: 'https://www.gstatic.com/generate_204', timeoutMs: 5000 }) })
      setNotice({ message: `${result.delayMs} ms`, severity: 'success' })
      await load('nodes')
    } catch (error) { setNotice({ message: error instanceof Error ? error.message : '测速失败', severity: 'error' }) }
    finally { setTestBusy(null) }
  }
  const selectNode = (node: NodeItem) => run(`select:${node.id}`, () => api(`/proxy-groups/PROXY/selection`, { method: 'POST', body: JSON.stringify({ name: node.name }) }), '节点已切换', async () => { await load('nodes') })
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
    {[['subscriptions', '订阅', <AutorenewRounded key="s"/>], ['nodes', '节点', <HubOutlined key="n"/>], ['output', '统一出口', <LinkRounded key="o"/>]] .map(([key, title, icon]) => <Button key={key as string} startIcon={icon} onClick={() => setPage(key as Page)} className={`nav-item ${page === key ? 'selected' : ''}`} fullWidth>{title}</Button>)}
  </Stack>

  if (authError) return <Box sx={{ minHeight: '100vh', display: 'grid', placeItems: 'center' }}><Stack alignItems="center" spacing={2}><Typography>{authError}</Typography><Button variant="contained" onClick={() => window.location.replace('/auth/login')}>重新登录</Button></Stack></Box>
  if (!authReady) return <Box sx={{ minHeight: '100vh', display: 'grid', placeItems: 'center' }}><CircularProgress size={24}/></Box>

  return <Box className="app-shell">
    {!mobile && <Drawer variant="permanent" className="side-drawer" PaperProps={{ className: 'drawer-paper' }}><Box className="brand"><span className="brand-mark"><WifiTetheringRounded/></span><Typography variant="subtitle1" fontWeight={750}>CLASH CLOUD</Typography></Box><Divider sx={{ borderColor: 'rgba(255,255,255,.12)' }}/>{nav}<Box className="drawer-bottom"><span className="live-dot"/> Mihomo 管理</Box></Drawer>}
    <Box component="main" className="main-area">
      {mobile && <AppBar position="sticky" elevation={0} className="mobile-bar"><Toolbar><span className="brand-mark"><WifiTetheringRounded/></span><Typography fontWeight={750}>CLASH CLOUD</Typography></Toolbar>{nav}</AppBar>}
      <Box className="content">
        <Box className="page-heading"><Box><Typography variant="h4">{{ subscriptions: '订阅', nodes: '节点', output: '统一出口' }[page]}</Typography></Box><Stack direction="row" spacing={1} alignItems="center">{page === 'subscriptions' && <Button variant="contained" startIcon={<AddRounded/>} onClick={() => { setActiveSub(null); setDialog('create') }}>添加订阅</Button>}{page === 'nodes' && <Tooltip title="刷新"><IconButton onClick={() => void load('nodes')}><AutorenewRounded/></IconButton></Tooltip>}<Tooltip title="退出登录"><IconButton onClick={() => void logout()} aria-label="退出登录"><LogoutRounded/></IconButton></Tooltip></Stack></Box>

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
          <Paper className="table-paper" variant="outlined"><TableContainer><Table aria-label="节点列表"><TableHead><TableRow><TableCell>节点</TableCell><TableCell>来源</TableCell><TableCell>协议</TableCell><TableCell align="right">延迟</TableCell><TableCell>状态</TableCell><TableCell align="right">操作</TableCell></TableRow></TableHead><TableBody>
            {loading && !nodes.length ? <TableRow><TableCell colSpan={6}><LoadingRows/></TableCell></TableRow> : nodes.map(node => <TableRow key={node.id} hover>
              <TableCell><Typography fontWeight={650}>{node.name}</Typography></TableCell><TableCell>{node.sourceName}</TableCell><TableCell><Chip size="small" label={node.type}/></TableCell><TableCell align="right" className="numeric">{node.latencyMs == null ? '—' : `${node.latencyMs} ms`}</TableCell><TableCell><Status status={node.available === false ? 'failed' : node.available === true ? 'healthy' : 'pending'}/></TableCell><TableCell align="right"><Stack direction="row" spacing={0.5} justifyContent="flex-end"><Tooltip title="测速"><span><IconButton size="small" aria-label={`测试 ${node.name}`} disabled={testBusy === node.id} onClick={() => void testNode(node)}>{testBusy === node.id ? <CircularProgress size={18}/> : <NetworkCheckRounded fontSize="small"/>}</IconButton></span></Tooltip><Button size="small" variant={node.selected ? 'outlined' : 'text'} disabled={!!node.selected || busy === `select:${node.id}`} onClick={() => void selectNode(node)}>{node.selected ? '当前使用' : '切换'}</Button></Stack></TableCell>
            </TableRow>)}
            {!loading && !nodes.length && <TableRow><TableCell colSpan={6}><Empty title="没有匹配的节点"/></TableCell></TableRow>}
          </TableBody></Table></TableContainer></Paper>
        </>}

        {page === 'output' && <OutputPanel output={output} loading={loading} onRefresh={() => void load('output')} onCreate={() => { setActiveLink(null); setLinkDialog('create') }} onCopy={link => void copyOutputLink(link)} onRename={link => { setActiveLink(link); setLinkDialog('rename') }} onToggle={link => void toggleOutputLink(link)} busy={busy}/>}
      </Box>
    </Box>
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
      <Box className="output-meta"><Meta label="节点数" value={String(output?.nodeCount ?? 0)}/><Meta label="配置版本" value={output?.revision ? `#${output.revision}` : '—'}/><Meta label="生成时间" value={fmtDate(output?.generatedAt)}/><Meta label="代理组" value={output?.selectedGroup || 'PROXY'}/></Box>
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
