// Экраны и состояния демо-сборки: адрес в приложении и шаги, которые
// приводят экран в нужное состояние (диалоги, поля F-86 — переходные).
// Один список для страницы-оглавления (#/demo) и выгрузки для дизайна
// (web/scripts/design-export.mjs): ссылка `?demo=<id>`.
import type { CardMode, HistoryEdit } from '@/api/types'
import type { RecordDTO } from '@/core/types'

import { EMPTY_DASHBOARD, type DemoAuth, type Fixtures } from './server'

/** Группы экранов — как в Design System pane (маркер @dsCard). */
export const DEMO_GROUPS = ['Вход', 'Списки', 'Формы', 'Диалоги', 'Дерево'] as const
export type DemoGroup = (typeof DEMO_GROUPS)[number]

/** Элемент страницы: первый `sel`, в тексте которого есть `text`; inner — внутри него. */
export interface Target {
  sel: string
  text?: string
  inner?: string
}

export type Step =
  | ({ do: 'click' | 'wait' | 'blur' | 'scroll' | 'upload' } & Target)
  | ({ do: 'fill'; value: string } & Target)
  /** следующий изменяющий запрос API ответит этой ошибкой */
  | { do: 'fault'; status: number; body: Record<string, unknown> }
  /** остановить анимации (контур «сохранено» затухает за 3 с) */
  | { do: 'freeze' }

export interface DemoState {
  id: string
  group: DemoGroup
  title: string
  /** что показывает: требование, ради которого состояние снято */
  note?: string
  /** адрес в приложении (маршрут) */
  path: string
  auth?: DemoAuth
  prefs?: HistoryEdit
  /** режим карточек аккаунта (F-108) */
  cardMode?: CardMode
  steps?: Step[]
}

function nameOf(r: RecordDTO): string {
  const v = r.fields.name?.values[0]?.value
  return typeof v === 'string' ? v : ''
}

/** Состояния по снимку демо-данных: id записей и групп — из него. */
export function demoStates(fx: Fixtures): DemoState[] {
  const D = fx.dashboard.id
  const live = fx.records.filter(r => !r.deleted)
  const id = (pred: (r: RecordDTO) => boolean, what: string) => {
    const r = live.find(pred)
    if (!r) throw new Error(`демо: в снимке нет записи «${what}»`)
    return r.id
  }
  const liza = id(r => r.template === 'kin.core.person' && nameOf(r) === 'Лиза', 'Лиза')
  const pair = id(r => r.template === 'kin.core.relationship' && !r.end, 'Пара')
  const wedding = id(r => r.template === 'kin.core.wedding', 'Свадьба')
  const org = id(r => r.template === 'kin.core.organization', 'Организация')
  const trashed = fx.records.find(r => r.deleted)?.id ?? liza
  const family = fx.groups.find(g => g.name === 'Семья')?.id ?? ''
  const lebedev = fx.groups.find(g => g.name === 'Лебедевы')?.id ?? ''

  const card = `/d/${D}/r/${liza}`
  const edit: Step = { do: 'click', sel: 'button.mode' }
  const row = (label: string, inner = 'input'): Target => ({ sel: '.field-row', text: label, inner })
  const phone = row('Телефон')

  return [
    { id: 'setup', group: 'Вход', title: 'Первичная настройка по токену', path: '/setup', auth: 'setup' },
    { id: 'login', group: 'Вход', title: 'Вход', path: '/login', auth: 'guest' },
    {
      id: 'login-error', group: 'Вход', title: 'Ошибка входа', path: '/login', auth: 'guest',
      steps: [
        { do: 'fill', sel: 'input[autocomplete="username"]', value: 'demo' },
        { do: 'fill', sel: 'input[autocomplete="current-password"]', value: 'неверный-пароль' },
        { do: 'click', sel: 'button[type="submit"]' },
        { do: 'wait', sel: '.kin-error' },
      ],
    },
    { id: 'account', group: 'Вход', title: 'Аккаунт: смена пароля и настройки', path: '/account' },
    {
      id: 'account-remembered', group: 'Вход', title: 'Аккаунт: запомненный выбор и его сброс', note: 'F-80', path: '/account',
      prefs: 'replace',
    },
    {
      id: 'account-password-error', group: 'Вход', title: 'Аккаунт: ошибка смены пароля', path: '/account',
      steps: [
        { do: 'fill', sel: 'input[autocomplete="current-password"]', value: 'не-тот-пароль' },
        { do: 'fill', sel: 'input[autocomplete="new-password"]', value: 'новый-пароль-1' },
        { do: 'fill', sel: 'form .kin-label:last-of-type input', value: 'новый-пароль-1' },
        { do: 'click', sel: 'form button[type="submit"]' },
        { do: 'wait', sel: 'form .kin-error' },
      ],
    },

    { id: 'dashboard-person', group: 'Списки', title: 'Дашборд: люди', path: `/d/${D}?e=kin.core.person` },
    { id: 'dashboard-organization', group: 'Списки', title: 'Дашборд: организации', path: `/d/${D}?e=kin.core.organization` },
    { id: 'dashboard-relationship', group: 'Списки', title: 'Дашборд: отношения', path: `/d/${D}?e=kin.core.relationship` },
    { id: 'dashboard-event', group: 'Списки', title: 'Дашборд: события', path: `/d/${D}?e=kin.core.event` },
    { id: 'search', group: 'Списки', title: 'Поиск', path: `/d/${D}?e=kin.core.person&q=${encodeURIComponent('Василь')}` },
    {
      id: 'search-nothing', group: 'Списки', title: 'Поиск: ничего не найдено',
      path: `/d/${D}?e=kin.core.person&q=${encodeURIComponent('Зинаида')}`,
    },
    { id: 'group-filter', group: 'Списки', title: 'Фильтр по группе «Семья»', path: `/d/${D}?e=kin.core.person&g=${family}` },
    { id: 'tree', group: 'Дерево', title: 'Дерево: всё полотно', note: 'F-110, F-112, F-116', path: `/d/${D}?v=tree`, steps: [{ do: 'wait', sel: '.tn-person' }] },
    {
      id: 'tree-group', group: 'Дерево', title: 'Дерево: фильтр по группе «Лебедевы»', note: 'F-110', path: `/d/${D}?v=tree&g=${lebedev}`,
      steps: [{ do: 'wait', sel: '.tn-person' }],
    },
    {
      id: 'tree-hide-dead', group: 'Дерево', title: 'Дерево: «Скрыть умерших» — развилки', note: 'F-119', path: `/d/${D}?v=tree&g=${lebedev}`,
      steps: [{ do: 'wait', sel: '.tn-person' }, { do: 'click', sel: '.tree-bar label', text: 'Скрыть умерших' }, { do: 'wait', sel: '.tn-dot' }],
    },
    {
      id: 'tree-card', group: 'Дерево', title: 'Дерево: карточка поверх дерева', note: 'F-113', path: `/d/${D}?v=tree&g=${lebedev}`,
      steps: [{ do: 'wait', sel: '.tn-person' }, { do: 'click', sel: '.tn-person', text: 'Ольга', inner: '.tn-name' }, { do: 'wait', sel: '.tree-card .record' }],
    },
    {
      id: 'tree-menu', group: 'Дерево', title: 'Дерево: меню (+) узла', note: 'F-73', path: `/d/${D}?v=tree`,
      steps: [{ do: 'wait', sel: '.tn-person' }, { do: 'click', sel: '.tn-person', text: 'Лиза', inner: '.tn-plus' }, { do: 'wait', sel: '.tree-menu' }],
    },
    { id: 'dashboard-empty', group: 'Списки', title: 'Пустой дашборд', path: `/d/${EMPTY_DASHBOARD}` },
    { id: 'trash', group: 'Списки', title: 'Корзина', path: `/d/${D}/trash` },
    { id: 'audit', group: 'Списки', title: 'Журнал изменений', path: `/d/${D}/audit` },
    { id: 'groups', group: 'Списки', title: 'Группы', note: 'F-105', path: `/d/${D}/groups` },
    {
      id: 'group-edit', group: 'Диалоги', title: 'Группа: «Входит в» и «Включает в себя»', note: 'F-105', path: `/d/${D}/groups`,
      steps: [
        { do: 'click', sel: '.tree li', text: 'Петровы', inner: 'button' }, { do: 'wait', sel: '.kin-popover .edit-parents' },
      ],
    },

    { id: 'card-read', group: 'Формы', title: 'Карточка: режим чтения', note: 'F-85, F-69, F-61', path: card },
    { id: 'card-edit', group: 'Формы', title: 'Карточка: режим правки', note: 'F-85, F-61', path: card, steps: [edit] },
    { id: 'card-form', group: 'Формы', title: 'Режим карточек «Форма»: сразу правка', note: 'F-108', path: card, cardMode: 'form' },
    {
      id: 'card-inline', group: 'Формы', title: 'Режим карточек «По месту»: поле открыто ручкой', note: 'F-108', path: card,
      cardMode: 'inline',
      steps: [{ do: 'click', ...row('Телефон', '.handle') }, { do: 'wait', ...phone }],
    },
    {
      id: 'field-dirty', group: 'Формы', title: 'Поле: изменено (жёлтый контур)', note: 'F-86', path: card,
      steps: [edit, { do: 'fill', ...phone, value: '+7 900 111-22-44' }],
    },
    {
      id: 'field-saved', group: 'Формы', title: 'Поле: сохранено (салатовый контур)', note: 'F-86', path: card,
      steps: [
        edit, { do: 'fill', ...phone, value: '+7 900 111-22-44' }, { do: 'blur', ...phone },
        { do: 'wait', sel: '.cell[data-state="saved"]' }, { do: 'freeze' },
      ],
    },
    {
      id: 'field-error', group: 'Формы', title: 'Поле: не сохранилось (красный контур)', note: 'F-86', path: card,
      steps: [
        edit,
        { do: 'fault', status: 400, body: { error: 'validation', field: 'phone', message: 'Телефон: неверный номер' } },
        { do: 'fill', ...phone, value: '+7 900 000' }, { do: 'blur', ...phone },
        { do: 'wait', sel: '.cell[data-state="error"]' }, { do: 'freeze' },
      ],
    },
    {
      id: 'history-ask', group: 'Формы', title: 'Вопрос «заменить или добавить в историю»', note: 'F-80', path: card,
      steps: [
        edit, { do: 'fill', ...row('Фамилия'), value: 'Новикова' }, { do: 'blur', ...row('Фамилия') },
        { do: 'wait', sel: '.history-choice' },
      ],
    },
    {
      id: 'history-popover', group: 'Формы', title: 'История значения поля', note: 'F-11, F-14', path: card,
      steps: [{ do: 'click', ...row('Фамилия', '.history-trigger') }, { do: 'wait', sel: '.kin-popover.history' }],
    },
    {
      id: 'history-stage', group: 'Формы', title: 'История этапов отношения', note: 'F-11', path: `/d/${D}/r/${pair}`,
      steps: [{ do: 'click', sel: '.stage-row .history-trigger' }, { do: 'wait', sel: '.kin-popover.history' }],
    },
    {
      id: 'relations', group: 'Формы', title: 'Раздел «Связи» в карточке', note: 'F-89', path: card,
      steps: [{ do: 'scroll', sel: '.relations' }],
    },
    { id: 'card-relationship', group: 'Формы', title: 'Карточка отношения «Пара» (этапы, участники)', path: `/d/${D}/r/${pair}` },
    { id: 'card-event', group: 'Формы', title: 'Карточка события «Свадьба»', path: `/d/${D}/r/${wedding}` },
    { id: 'card-organization', group: 'Формы', title: 'Карточка организации', path: `/d/${D}/r/${org}` },
    { id: 'card-trashed', group: 'Формы', title: 'Карточка записи в корзине', note: 'F-44', path: `/d/${D}/r/${trashed}` },
    {
      id: 'add-field', group: 'Формы', title: 'Добавить скрытое поле раздела', note: 'F-61 (show: filled)', path: card,
      steps: [edit, { do: 'click', sel: '.add-field' }, { do: 'wait', sel: '.add-list' }],
    },
    {
      id: 'draft', group: 'Формы', title: 'Черновик новой записи', note: 'F-88', path: `/d/${D}?e=kin.core.person`,
      steps: [
        { do: 'click', sel: 'button.create' }, { do: 'wait', sel: '.kin-dialog .templates' },
        { do: 'click', sel: '.kin-dialog .templates button', text: 'Человек' }, { do: 'wait', sel: '.draft-hint' },
      ],
    },

    {
      id: 'create-relation', group: 'Диалоги', title: 'Создание связи', note: 'F-87', path: card,
      steps: [
        { do: 'click', sel: '.new-relation' }, { do: 'wait', sel: '.kin-popover .menu' },
        { do: 'click', sel: '.kin-popover .menu button', text: 'Дружба' }, { do: 'wait', sel: '.create-relation' },
      ],
    },
    {
      id: 'create-relation-new', group: 'Диалоги', title: 'Создание связи: быстрое создание участника', note: 'F-87', path: card,
      steps: [
        { do: 'click', sel: '.new-relation' }, { do: 'wait', sel: '.kin-popover .menu' },
        { do: 'click', sel: '.kin-popover .menu button', text: 'Дружба' }, { do: 'wait', sel: '.create-relation' },
        { do: 'click', sel: '.create-relation button.new:not([disabled])' }, { do: 'wait', sel: '.new-participant' },
        { do: 'fill', sel: '.new-participant', inner: 'input', value: 'Мария' },
      ],
    },
    {
      id: 'quick-add-menu', group: 'Диалоги', title: 'Меню (+) карточки', note: 'F-73, F-43', path: card,
      steps: [{ do: 'click', sel: '.quick-add' }, { do: 'wait', sel: '.kin-popover .menu' }],
    },
    {
      id: 'quick-add-form', group: 'Диалоги', title: 'Быстрая форма из меню (+)', note: 'F-73', path: card,
      steps: [
        { do: 'click', sel: '.quick-add' }, { do: 'wait', sel: '.kin-popover .menu' },
        { do: 'click', sel: '.kin-popover .menu button:not([disabled])' }, { do: 'wait', sel: '.quick-form' },
      ],
    },
    {
      id: 'crop', group: 'Диалоги', title: 'Обрезка фото по местам показа', note: 'F-72, F-79', path: card,
      steps: [
        edit, { do: 'click', ...row('Фото', '.upload') }, { do: 'wait', sel: '.image-pick' },
        { do: 'upload', sel: '.image-pick input[type="file"]' }, { do: 'wait', sel: '.crop-dialog img' },
      ],
    },
    {
      id: 'image-pick', group: 'Диалоги', title: 'Загрузка изображения: буфер, перетаскивание, файл', note: 'F-107', path: card,
      steps: [edit, { do: 'click', ...row('Фото', '.upload') }, { do: 'wait', sel: '.image-pick .drop' }],
    },
    {
      id: 'dashboard-settings', group: 'Диалоги', title: 'Настройки дашборда: шаблоны, поля быстрой формы', note: 'F-132, F-124',
      path: `/d/${D}`,
      steps: [
        { do: 'click', sel: 'button[aria-label="Настройки дашборда"]' }, { do: 'wait', sel: '.settings [role=tab]' },
        { do: 'click', sel: '.settings [role=tab]:nth-child(2)' }, { do: 'wait', sel: '.settings .cloud .pill' },
      ],
    },
    {
      id: 'create-groups', group: 'Диалоги', title: 'Создание записи: шаблон и группы', note: 'F-04, F-81',
      path: `/d/${D}?e=kin.core.person&g=${family}`,
      steps: [
        { do: 'click', sel: 'button.create' }, { do: 'wait', sel: '.kin-dialog .create-groups' },
        { do: 'click', sel: '.kin-dialog .choose' }, { do: 'wait', sel: '.kin-popover .tree' },
      ],
    },
  ]
}
