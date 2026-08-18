/** 终端里保留的 SGR 样式。其它转义（光标、清屏、标题）仍然丢掉。 */

export type TermStyle = {
  fg?: string
  bg?: string
  bold?: boolean
  dim?: boolean
  underline?: boolean
  inverse?: boolean
}

export type TermRun = { text: string; style: TermStyle }

export type TermBuf = {
  runs: TermRun[]
  style: TermStyle
}

const MAX_CHARS = 200_000

// 深色底上能分清的 16 色，接近常见 xterm 调色。
const PALETTE = [
  '#4b5563',
  '#f87171',
  '#4ade80',
  '#facc15',
  '#60a5fa',
  '#c084fc',
  '#22d3ee',
  '#e5e7eb',
  '#9ca3af',
  '#fca5a5',
  '#86efac',
  '#fde047',
  '#93c5fd',
  '#d8b4fe',
  '#67e8f9',
  '#f9fafb',
]

const CUBE = [0, 95, 135, 175, 215, 255]

export function newTermBuf(): TermBuf {
  return { runs: [], style: {} }
}

export function clearTermBuf(buf: TermBuf) {
  buf.runs = []
  buf.style = {}
}

export function appendAnsi(buf: TermBuf, chunk: string) {
  const s = chunk.replace(/\r\n/g, '\n')
  let i = 0
  while (i < s.length) {
    const c = s.charCodeAt(i)
    if (c === 0x1b) {
      i = consumeEsc(s, i, buf)
      continue
    }
    if (c === 0x08 || c === 0x7f) {
      backspace(buf)
      i++
      continue
    }
    if (c === 0x0d) {
      carriage(buf)
      i++
      continue
    }
    if (c === 0x0a || c === 0x09) {
      pushText(buf, s[i])
      i++
      continue
    }
    if (c < 32 || (c >= 0x80 && c <= 0x9f) || c === 0xfffd) {
      i++
      continue
    }
    let j = i + 1
    while (j < s.length) {
      const d = s.charCodeAt(j)
      if (d === 0x1b || d < 32 || (d >= 0x80 && d <= 0x9f) || d === 0xfffd) break
      j++
    }
    pushText(buf, s.slice(i, j))
    i = j
  }
  trim(buf)
}

export function renderAnsi(buf: TermBuf): string {
  let html = ''
  for (const r of buf.runs) {
    const t = esc(r.text)
    const css = styleCss(r.style)
    html += css ? `<span style="${css}">${t}</span>` : t
  }
  return html
}

function consumeEsc(s: string, i: number, buf: TermBuf): number {
  const next = s[i + 1]
  if (next === '[') {
    const m = /^\[([0-?]*)([ -/]*)([@-~])/.exec(s.slice(i + 1))
    if (!m) return i + 2
    if (m[3] === 'm') applySgr(buf, m[1])
    return i + 1 + m[0].length
  }
  if (next === ']') {
    const rest = s.slice(i + 2)
    const endBell = rest.indexOf('\u0007')
    const endSt = rest.indexOf('\u001B\\')
    let end = -1
    let skip = 0
    if (endBell >= 0 && (endSt < 0 || endBell < endSt)) {
      end = endBell
      skip = 1
    } else if (endSt >= 0) {
      end = endSt
      skip = 2
    }
    if (end < 0) return s.length
    return i + 2 + end + skip
  }
  return Math.min(i + 2, s.length)
}

function applySgr(buf: TermBuf, raw: string) {
  const parts = raw.split(';')
  const params = parts.map((p) => (p === '' ? 0 : Number(p)))
  let st = { ...buf.style }
  for (let i = 0; i < params.length; i++) {
    const p = params[i]
    if (!Number.isFinite(p)) continue
    if (p === 0) {
      st = {}
      continue
    }
    if (p === 1) st.bold = true
    else if (p === 2) st.dim = true
    else if (p === 4) st.underline = true
    else if (p === 7) st.inverse = true
    else if (p === 22) {
      delete st.bold
      delete st.dim
    } else if (p === 24) delete st.underline
    else if (p === 27) delete st.inverse
    else if (p >= 30 && p <= 37) st.fg = PALETTE[p - 30]
    else if (p >= 90 && p <= 97) st.fg = PALETTE[p - 90 + 8]
    else if (p === 39) delete st.fg
    else if (p >= 40 && p <= 47) st.bg = PALETTE[p - 40]
    else if (p >= 100 && p <= 107) st.bg = PALETTE[p - 100 + 8]
    else if (p === 49) delete st.bg
    else if (p === 38 || p === 48) {
      const color = readExtColor(params, i)
      if (color) {
        if (p === 38) st.fg = color.value
        else st.bg = color.value
        i = color.next
      }
    }
  }
  buf.style = st
}

function readExtColor(params: number[], i: number): { value: string; next: number } | null {
  const mode = params[i + 1]
  if (mode === 5 && i + 2 < params.length) {
    return { value: color256(params[i + 2]), next: i + 2 }
  }
  if (mode === 2 && i + 4 < params.length) {
    const r = clampByte(params[i + 2])
    const g = clampByte(params[i + 3])
    const b = clampByte(params[i + 4])
    return { value: `rgb(${r},${g},${b})`, next: i + 4 }
  }
  return null
}

function color256(n: number): string {
  n = Math.max(0, Math.min(255, n | 0))
  if (n < 16) return PALETTE[n]
  if (n < 232) {
    const v = n - 16
    const r = CUBE[Math.floor(v / 36)]
    const g = CUBE[Math.floor((v % 36) / 6)]
    const b = CUBE[v % 6]
    return `rgb(${r},${g},${b})`
  }
  const g = 8 + (n - 232) * 10
  return `rgb(${g},${g},${g})`
}

function clampByte(n: number): number {
  if (!Number.isFinite(n)) return 0
  return Math.max(0, Math.min(255, n | 0))
}

function pushText(buf: TermBuf, text: string) {
  if (!text) return
  const last = buf.runs[buf.runs.length - 1]
  if (last && sameStyle(last.style, buf.style)) {
    last.text += text
    return
  }
  buf.runs.push({ text, style: { ...buf.style } })
}

function backspace(buf: TermBuf) {
  for (let i = buf.runs.length - 1; i >= 0; i--) {
    const t = buf.runs[i].text
    if (!t.length) continue
    if (t.endsWith('\n')) return
    buf.runs[i].text = t.slice(0, -1)
    if (!buf.runs[i].text) buf.runs.splice(i, 1)
    return
  }
}

function carriage(buf: TermBuf) {
  for (let i = buf.runs.length - 1; i >= 0; i--) {
    const at = buf.runs[i].text.lastIndexOf('\n')
    if (at >= 0) {
      buf.runs[i].text = buf.runs[i].text.slice(0, at + 1)
      buf.runs.length = i + 1
      if (buf.runs[i] && !buf.runs[i].text) buf.runs.pop()
      return
    }
  }
  buf.runs = []
}

function trim(buf: TermBuf) {
  let n = 0
  for (const r of buf.runs) n += r.text.length
  if (n <= MAX_CHARS) return
  let drop = n - MAX_CHARS
  while (drop > 0 && buf.runs.length) {
    const first = buf.runs[0]
    if (first.text.length <= drop) {
      drop -= first.text.length
      buf.runs.shift()
    } else {
      first.text = first.text.slice(drop)
      drop = 0
    }
  }
}

function sameStyle(a: TermStyle, b: TermStyle): boolean {
  return (
    a.fg === b.fg &&
    a.bg === b.bg &&
    a.bold === b.bold &&
    a.dim === b.dim &&
    a.underline === b.underline &&
    a.inverse === b.inverse
  )
}

function styleCss(s: TermStyle): string {
  let fg = s.fg
  let bg = s.bg
  if (s.inverse) {
    fg = s.bg || '#111318'
    bg = s.fg || '#d8dee9'
  }
  const parts: string[] = []
  if (fg) parts.push(`color:${fg}`)
  if (bg) parts.push(`background:${bg}`)
  if (s.bold) parts.push('font-weight:700')
  if (s.dim) parts.push('opacity:.65')
  if (s.underline) parts.push('text-decoration:underline')
  return parts.join(';')
}

function esc(s: string): string {
  return s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
}
