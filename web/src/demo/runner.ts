// Шаги состояния демо (states.ts) по живому DOM: клик, ввод, уход из поля,
// загрузка файла. После каждого шага — ожидание тишины API. Итог —
// атрибут data-demo-ready на <html>: «1» или текст ошибки (его ждёт выгрузка).
import type { DemoState, Step, Target } from './states'

const TIMEOUT = 5000

export interface RunnerEnv {
  /** запросов API в работе */
  pending: () => number
  /** файл для загрузки (шаг upload) */
  file: () => File
  fault: (status: number, body: Record<string, unknown>) => void
}

const sleep = (ms: number) => new Promise(r => setTimeout(r, ms))
const frame = () => new Promise(r => requestAnimationFrame(() => r(null)))

function find(t: Target): HTMLElement | null {
  for (const el of document.querySelectorAll<HTMLElement>(t.sel)) {
    if (t.text && !(el.textContent ?? '').includes(t.text)) continue
    if (!t.inner) return el
    const inner = el.querySelector<HTMLElement>(t.inner)
    if (inner) return inner
  }
  return null
}

async function waitFor(t: Target): Promise<HTMLElement> {
  const until = Date.now() + TIMEOUT
  for (;;) {
    const el = find(t)
    if (el) return el
    if (Date.now() > until) throw new Error(`нет элемента ${t.sel}${t.text ? ` «${t.text}»` : ''}${t.inner ? ` → ${t.inner}` : ''}`)
    await sleep(50)
  }
}

/** Тишина: нет запросов API 150 мс подряд. */
async function idle(env: RunnerEnv): Promise<void> {
  const until = Date.now() + TIMEOUT
  let quiet = 0
  while (quiet < 150 && Date.now() < until) {
    await sleep(25)
    quiet = env.pending() ? 0 : quiet + 25
  }
  await frame()
  await frame()
}

/** Ввод как с клавиатуры: значение через сеттер элемента и событие input. */
function fill(el: HTMLElement, value: string) {
  el.focus()
  if (el instanceof HTMLInputElement || el instanceof HTMLTextAreaElement) {
    const proto = el instanceof HTMLInputElement ? HTMLInputElement.prototype : HTMLTextAreaElement.prototype
    Object.getOwnPropertyDescriptor(proto, 'value')?.set?.call(el, value)
  }
  el.dispatchEvent(new Event('input', { bubbles: true }))
  el.dispatchEvent(new Event('change', { bubbles: true }))
}

async function step(s: Step, env: RunnerEnv): Promise<void> {
  switch (s.do) {
    case 'fault':
      env.fault(s.status, s.body)
      return
    case 'freeze': {
      const st = document.createElement('style')
      st.dataset.demo = 'freeze'
      st.textContent = '*, *::before, *::after { animation-play-state: paused !important; }'
      document.head.append(st)
      return
    }
  }
  const el = await waitFor(s)
  switch (s.do) {
    case 'wait':
      return
    case 'click':
      // открываемое окно (всплывающая форма) — у видимого места экрана
      el.scrollIntoView({ block: 'center' })
      // вкладки Reka UI переключаются по mousedown, не по click
      if (el.getAttribute('role') === 'tab') el.dispatchEvent(new MouseEvent('mousedown', { bubbles: true, button: 0 }))
      el.click()
      return
    case 'fill':
      fill(el, s.value)
      return
    case 'blur':
      el.focus()
      el.blur()
      return
    case 'scroll':
      el.scrollIntoView({ block: 'start' })
      return
    case 'upload': {
      if (!(el instanceof HTMLInputElement)) throw new Error('upload: нужен input[type=file]')
      const dt = new DataTransfer()
      dt.items.add(env.file())
      el.files = dt.files
      el.dispatchEvent(new Event('change', { bubbles: true }))
    }
  }
}

export async function runState(state: DemoState | undefined, env: RunnerEnv): Promise<void> {
  const root = document.documentElement
  try {
    await idle(env)
    for (const s of state?.steps ?? []) {
      await step(s, env)
      await idle(env)
    }
    root.dataset.demoReady = '1'
  } catch (e) {
    root.dataset.demoReady = `error: ${e instanceof Error ? e.message : String(e)}`
    console.error('демо:', e)
  }
}
