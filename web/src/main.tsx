import React from 'react'
import ReactDOM from 'react-dom/client'
import { CssBaseline, ThemeProvider, createTheme } from '@mui/material'
import App from './App'
import './style.css'

const theme = createTheme({
  palette: { mode: 'light', primary: { main: '#d58a28', contrastText: '#171b22' }, background: { default: '#f7f5ef', paper: '#fffefa' }, text: { primary: '#202630', secondary: '#69717b' }, success: { main: '#48816b' }, warning: { main: '#b87525' }, error: { main: '#aa544a' }, divider: '#e6e1d7' },
  shape: { borderRadius: 10 }, typography: { fontFamily: 'Inter, "Noto Sans SC", "PingFang SC", "Microsoft YaHei", sans-serif', button: { textTransform: 'none', fontWeight: 600 }, h4: { fontWeight: 700, letterSpacing: '-.04em' } },
  components: { MuiButton: { styleOverrides: { root: { borderRadius: 8, minHeight: 38, boxShadow: 'none' } } }, MuiTableCell: { styleOverrides: { head: { color: '#69717b', fontWeight: 600, background: '#faf9f5' }, root: { borderColor: '#eeeae2' } } }, MuiPaper: { styleOverrides: { root: { backgroundImage: 'none' } } } },
})

ReactDOM.createRoot(document.getElementById('root')!).render(<React.StrictMode><ThemeProvider theme={theme}><CssBaseline/><App/></ThemeProvider></React.StrictMode>)
