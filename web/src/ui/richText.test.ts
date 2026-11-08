import { describe, expect, it } from 'vitest'

import { renderRichText, sanitizeHtml } from './richText'

describe('форматированный текст (F-141)', () => {
  it('Markdown — с форматированием', () => {
    const html = renderRichText('# Детство\n\n**Родился** в _Горках_.\n\n- раз\n- два')
    expect(html).toContain('<h1>Детство</h1>')
    expect(html).toContain('<strong>Родился</strong>')
    expect(html).toContain('<em>Горках</em>')
    expect(html).toContain('<li>раз</li>')
  })

  it('безопасный HTML остаётся, опасный — нет', () => {
    const html = renderRichText('<u>подчёркнуто</u><script>alert(1)</script><img src=x onerror="alert(1)"><b onclick="x()">жирно</b>')
    expect(html).toContain('<u>подчёркнуто</u>')
    expect(html).toContain('<b>жирно</b>')
    expect(html).not.toMatch(/script|onerror|onclick|<img/i)
  })

  it('ссылки: только безопасные схемы, в новой вкладке', () => {
    const ok = renderRichText('[сайт](https://example.org) [почта](mailto:a@b.c)')
    expect(ok).toContain('href="https://example.org"')
    expect(ok).toContain('target="_blank"')
    expect(ok).toContain('rel="noopener noreferrer nofollow"')
    expect(ok).toContain('href="mailto:a@b.c"')
    for (const bad of ['[x](javascript:alert(1))', '<a href="javascript:alert(1)">x</a>', '<a href=" jav&#x09;ascript:alert(1)">x</a>',
      '<a href="data:text/html,<script>alert(1)</script>">x</a>', '<a href="vbscript:x">x</a>', '[x](foo1:bar)', '<a href="steam:run">x</a>']) {
      expect(renderRichText(bad)).not.toMatch(/href/i)
    }
    expect(renderRichText('[отн](page.html) [якорь](#a) [путь](/d/x)')).toMatch(/href="page.html".*href="#a".*href="\/d\/x"/)
  })

  it('стили, iframe, svg, формы и атрибуты событий убираются', () => {
    const html = sanitizeHtml('<p style="color:red" class="x" id="y">т</p><iframe src="https://e.org"></iframe><svg onload="x()"></svg><form><input></form>')
    expect(html).toBe('<p>т</p>')
  })
})

describe('сохранение форматированного текста', () => {
  it('отступ первой строки (блок кода) сохраняется, пустые строки в начале и пробелы в конце — нет', async () => {
    const { normalize, sameValue } = await import('./values')
    expect(normalize('\n\n    код\n  ещё\n\n', 'rich_text')).toBe('    код\n  ещё')
    expect(normalize('  текст  ')).toBe('текст')
    expect(sameValue('    код', 'код', 'rich_text')).toBe(false)
  })
})
