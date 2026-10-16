// Модель ракурса «Дерево» (F-110…F-119): что раскладке нужно знать о людях
// и связях. Строится из записей по данным паков (graph.ts), раскладывается
// layout.ts; от Vue и Vue Flow не зависит.

/** Человек — узел дерева. */
export interface TreePerson {
  id: string
  /** год рождения — для поздних детей (F-115) и порядка */
  year?: number
  /** умер: дата смерти или признак `end_flag` (F-120, F-126) */
  dead?: boolean
}

/** Пара (`as: couple`): двое рядом, узел отношения между ними (F-112). */
export interface TreeCouple {
  id: string
  /** запись отношения, если id узла не она (пара из 3+ участников: `rel:N`) */
  relation?: string
  a: string
  b: string
  /** ключ порядка: раньше — левее (F-112) */
  order: string
  label: string
}

/** «Брат / сестра» (`as: siblings`): узел над участниками. */
export interface TreeSiblings {
  id: string
  members: string[]
  label: string
}

/** «Родитель — ребёнок» (`as: descent`): родитель выше ребёнка. */
export interface TreeDescent {
  id: string
  parent: string
  child: string
  /** вид линии варианта (`tree_line: dashed`, F-118) */
  dashed?: boolean
  /** название варианта — подпись линии от пары («Приёмный») */
  kind?: string
  /** форма роли родителя — подпись отдельной линии («Отчим») */
  label?: string
}

export interface TreeInput {
  persons: TreePerson[]
  couples: TreeCouple[]
  siblings: TreeSiblings[]
  descents: TreeDescent[]
}

/** Узел раскладки; x, y — левый верхний угол. */
export interface TreeNode {
  id: string
  type: 'person' | 'ref' | 'union' | 'siblings' | 'dot'
  x: number
  y: number
  w: number
  /** высота плашки подписи (union, siblings); у людей — по содержимому */
  h?: number
  row: number
  /** человек узла (person, ref) */
  person?: string
  /** отношение узла (union, siblings) */
  relation?: string
  label?: string
  /** развилка нескольких скрытых — «•••» (F-119) */
  many?: boolean
  /** отсылка: сосед по паре, рядом с которым она стоит, и подпись отношения (значок F-117) */
  near?: string
  nearLabel?: string
}

/** Линия: шина (вниз — вбок — вниз) или прямая между соседями ряда. */
export interface TreeEdge {
  id: string
  type: 'bus' | 'straight'
  source: string
  target: string
  sourceHandle: 'b' | 'r' | 'c'
  targetHandle: 't' | 'l'
  /** высота горизонтального отрезка шины */
  busY?: number
  /** обход промежуточных рядов: вниз до y, вбок до x (F-110, F-112) */
  via?: { x: number; y: number }
  dashed?: boolean
  label?: string
  /** семья или отношение шины — линии одного источника к ребёнку (F-127) */
  group?: string
  /** отношения, которые рисует линия: двойной клик открывает карточку (F-127) */
  rels?: string[]
}

export interface TreeLayout {
  nodes: TreeNode[]
  edges: TreeEdge[]
}
