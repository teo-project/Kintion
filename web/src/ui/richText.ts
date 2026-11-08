// Форматированный текст (F-141): значение — Markdown; HTML в нём допускается
// только безопасным подмножеством. При показе Markdown превращается в HTML
// (marked), и результат всегда очищается (DOMPurify): ни скриптов, ни
// обработчиков, ни стилей, ни ссылок со схемами кроме http(s), mailto, tel и
// относительных. Картинок нет: внешняя картинка — запрос третьей стороне.
import DOMPurify from 'dompurify'
import { marked } from 'marked'

/** Теги, которые остаются после очистки. */
const TAGS = [
  'p', 'br', 'hr', 'strong', 'b', 'em', 'i', 'u', 's', 'del', 'ins', 'mark', 'sub', 'sup', 'small',
  'h1', 'h2', 'h3', 'h4', 'h5', 'h6', 'blockquote', 'ul', 'ol', 'li', 'code', 'pre', 'a',
  'table', 'thead', 'tbody', 'tr', 'th', 'td', 'span', 'div',
]
const ATTRS = ['href', 'title', 'start']
/**
 * Допустимые адреса ссылок: http(s), mailto, tel, якорь и относительные.
 * Относительный адрес — без схемы: до первого «/», «?» или «#» нет «:».
 */
const URI = /^(?:(?:https?|mailto|tel):|[/?#.]|[^:/?#]*(?:[/?#]|$))/i

let hooked = false

/** Ссылки — в новой вкладке и без доступа к странице (noopener). */
function hook() {
  if (hooked) return
  hooked = true
  DOMPurify.addHook('afterSanitizeAttributes', node => {
    if (node.tagName === 'A' && node.getAttribute('href')) {
      node.setAttribute('target', '_blank')
      node.setAttribute('rel', 'noopener noreferrer nofollow')
    }
  })
}

/** Очистка готового HTML безопасным подмножеством. */
export function sanitizeHtml(html: string): string {
  hook()
  return DOMPurify.sanitize(html, {
    ALLOWED_TAGS: TAGS,
    ALLOWED_ATTR: ATTRS,
    ALLOWED_URI_REGEXP: URI,
    ALLOW_DATA_ATTR: false,
    ALLOW_ARIA_ATTR: false,
  })
}

/** Markdown → безопасный HTML для показа. */
export function renderRichText(src: string): string {
  const html = marked.parse(src, { async: false, gfm: true, breaks: true })
  return sanitizeHtml(html)
}

