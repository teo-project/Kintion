// Пути сохранения интерфейса: каждый — «сохранил → перезагрузил → видно».
// Поля карточки всех типов (свои и общие), шапка, быстрая форма, настройки
// дашборда, группы, выделение, режимы карточки. Работает и на свежей
// установке (настройка по токену), и после основного сценария (вход).
import { expect, test, type Locator, type Page } from '@playwright/test'

const TOKEN = process.env.KIN_E2E_TOKEN ?? ''
const LOGIN = 'owner'
/** пароль свежей установки; после основного сценария — сменённый */
const PASSWORDS = ['e2e-password-0.1', 'e2e-password-0.1-new']

/** PNG 64×48 */
const PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAEAAAAAwCAIAAAAuKetIAAAAQ0lEQVR42u3PQQkAAAgEsAtmMCMaywp+hcEKLNP1WgQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQErhYYG9EAD2sj5gAAAABJRU5ErkJggg==',
  'base64',
)

async function signIn(page: Page) {
  await page.goto('/')
  await expect(page).toHaveURL(/\/(setup|login|d\/)/)
  if (page.url().includes('/setup')) {
    await page.getByLabel('Токен первого запуска').fill(TOKEN)
    await page.getByLabel('Логин').fill(LOGIN)
    await page.getByLabel('Пароль', { exact: true }).fill(PASSWORDS[0]!)
    await page.getByLabel('Пароль ещё раз').fill(PASSWORDS[0]!)
    await page.getByRole('button', { name: 'Создать владельца' }).click()
  } else if (page.url().includes('/login')) {
    for (const p of PASSWORDS) {
      await page.getByLabel('Логин').fill(LOGIN)
      await page.getByLabel('Пароль', { exact: true }).fill(p)
      await page.getByRole('button', { name: 'Войти' }).click()
      if (await page.waitForURL(/\/d\//, { timeout: 3000 }).then(() => true, () => false)) break
    }
  }
  await expect(page).toHaveURL(/\/d\/[0-9a-f-]+/)
}

function fieldRow(scope: Page | Locator, name: string): Locator {
  return scope.locator('.field-row').filter({ has: scope.locator('.label', { hasText: new RegExp(`^${name}$`) }) })
}

async function editMode(page: Page, on: boolean) {
  const btn = page.getByRole('button', { name: on ? 'Редактировать' : 'Готово' })
  if (await btn.isVisible()) await btn.click()
  await expect(page.getByRole('button', { name: on ? 'Готово' : 'Редактировать' })).toBeVisible()
}

async function saved(scope: Locator) {
  // у поля с несколькими значениями сохранённое уходит в список, «сохранено» — у ввода нового
  await expect(scope.locator('.cell[data-state="saved"]').first()).toBeVisible()
}

/** Поле, скрытое пока пусто (show: filled), — показать через «Добавить поле». */
async function reveal(page: Page, field: string) {
  if (await fieldRow(page, field).isVisible()) return
  await page.getByRole('button', { name: 'Добавить поле' }).first().click()
  await page.locator('.add-list').getByRole('button', { name: field }).click()
  await expect(fieldRow(page, field)).toBeVisible()
}

/** Новая запись шаблона из «Списков»: первое поле создаёт её (F-88). */
async function createRecord(page: Page, tab: string, template: string | null, field: string, value: string): Promise<string> {
  await page.getByRole('link', { name: 'Списки', exact: true }).click()
  await page.getByRole('tab', { name: tab }).click()
  await page.getByRole('button', { name: 'Создать' }).click()
  // окно выбора — если шаблонов несколько или есть группы для предложения
  const dialog = page.getByRole('dialog', { name: 'Создать запись' })
  await expect(dialog.or(page.getByText('Новая запись появится')).first()).toBeVisible()
  if (await dialog.isVisible()) await dialog.getByRole('button', { name: template ?? tab, exact: true }).click()
  await expect(page).toHaveURL(/\/r\/draft-/)
  const input = fieldRow(page, field).getByRole('textbox', { name: field, exact: true })
  await input.fill(value)
  await input.press('Tab')
  await expect(page).toHaveURL(/\/r\/[0-9a-f]{8}-[0-9a-f-]+$/)
  return page.url()
}

/** После перезагрузки поле показывает значение (режим чтения). */
async function persisted(page: Page, field: string, text: string | RegExp) {
  await page.reload()
  await editMode(page, false)
  await expect(fieldRow(page, field).locator('.values')).toContainText(text)
}

/** Своё поле записи (общее — toAll) заданного типа. */
async function addCustom(page: Page, name: string, type: string, opts: { options?: string; toAll?: boolean } = {}) {
  await page.getByRole('button', { name: 'Своё поле' }).first().click()
  const pop = page.locator('.custom-form')
  await pop.getByLabel('Название').fill(name)
  await pop.getByLabel('Тип').selectOption({ label: type })
  if (opts.options) await pop.getByLabel('Варианты, по одному в строке').fill(opts.options)
  if (opts.toAll) await pop.getByRole('checkbox', { name: 'Добавить всем записям этого шаблона' }).check()
  await pop.getByRole('button', { name: 'Сохранить' }).click()
  await expect(pop).toBeHidden()
  await expect(fieldRow(page, name)).toBeVisible()
}

test.describe.configure({ mode: 'serial' })

test('пути сохранения: сохранил → перезагрузил → видно', async ({ page }) => {
  await signIn(page)
  const stamp = Date.now().toString(36)
  const surname = `Сохранов${stamp}`
  /** общее поле шаблона — одно на дашборд: имя уникально для повторных прогонов */
  const shared = `Общее ${stamp}`
  let person = ''

  await test.step('человек: текст, несколько значений, выбор, место, телефон, почта, ссылка', async () => {
    person = await createRecord(page, 'Человек', null, 'Имя', 'Пётр')
    const cases: { field: string; kind: 'text' | 'select'; value: string; shown?: string }[] = [
      { field: 'Фамилия', kind: 'text', value: surname },
      { field: 'Отчество', kind: 'text', value: 'Ильич' },
      { field: 'Ник', kind: 'text', value: 'Петя' },
      { field: 'Пол', kind: 'select', value: 'Мужской' },
      { field: 'Адрес', kind: 'text', value: 'Тверь' },
      { field: 'Телефон', kind: 'text', value: '+7 900 000-00-01' },
      { field: 'Email', kind: 'text', value: 'petr@example.org' },
      { field: 'Ссылки', kind: 'text', value: 'example.org' },
    ]
    for (const c of cases) {
      const row = fieldRow(page, c.field)
      await reveal(page, c.field)
      if (c.kind === 'select') {
        const sel = row.getByRole('combobox')
        await sel.selectOption({ label: c.value })
        await sel.press('Tab')
      } else {
        const input = row.getByRole('textbox').first()
        await input.fill(c.value)
        await input.press('Tab')
      }
      await saved(row)
    }
    await page.reload()
    await editMode(page, false)
    for (const c of cases) await expect(fieldRow(page, c.field).locator('.values'), c.field).toContainText(c.shown ?? c.value)
  })

  await test.step('шапка: даты и «Умер» (да/нет — сегменты)', async () => {
    await editMode(page, true)
    const born = page.locator('.sys-row').filter({ hasText: 'Дата рождения' })
    await born.getByRole('textbox', { name: 'Год' }).fill('1950')
    await born.getByRole('textbox', { name: 'Год' }).press('Tab')
    await saved(born)
    // «Умер» — пока нет даты смерти (с датой флаг не нужен и скрыт)
    await page.getByRole('button', { name: 'Добавить дату' }).click()
    await page.getByRole('button', { name: 'Умер' }).click()
    const dead = page.locator('.basics').locator('.field-row').filter({ hasText: 'Умер' })
    await dead.getByRole('button', { name: 'Да' }).click()
    await saved(dead)
    await page.reload()
    await editMode(page, false)
    await expect(page.locator('.sys-row').filter({ hasText: 'Дата рождения' })).toContainText('1950')
    await expect(page.locator('.basics').locator('.field-row').filter({ hasText: 'Умер' })).toContainText('Да')
    await editMode(page, true)
    await page.getByRole('button', { name: 'Добавить дату' }).click()
    await page.getByRole('button', { name: 'Дата смерти' }).click()
    const died = page.locator('.sys-row').filter({ hasText: 'Дата смерти' })
    await died.getByRole('textbox', { name: 'Год' }).fill('2020')
    await died.getByRole('textbox', { name: 'Год' }).press('Tab')
    await saved(died)
    await page.reload()
    await editMode(page, false)
    await expect(page.locator('.sys-row').filter({ hasText: 'Дата смерти' })).toContainText('2020')
  })

  await test.step('свои поля всех типов и общее поле', async () => {
    await editMode(page, true)
    // число, дата, да/нет, выбор, выбор или своё (вариант из списка), ссылка, изображение, общее
    await addCustom(page, 'Рост', 'Число')
    let row = fieldRow(page, 'Рост')
    await row.getByRole('spinbutton').fill('182')
    await row.getByRole('spinbutton').press('Tab')
    await saved(row)

    await addCustom(page, 'Крещение', 'Дата')
    row = fieldRow(page, 'Крещение')
    await row.getByRole('textbox', { name: 'Год' }).fill('1951')
    await row.getByRole('textbox', { name: 'Год' }).press('Tab')
    await saved(row)

    await addCustom(page, 'Ветеран', 'Да / нет')
    row = fieldRow(page, 'Ветеран')
    await row.getByRole('button', { name: 'Нет' }).click()
    await saved(row)

    await addCustom(page, 'Группа крови', 'Выбор', { options: 'Первая\nВторая' })
    row = fieldRow(page, 'Группа крови')
    await row.getByRole('combobox').selectOption({ label: 'Вторая' })
    await row.getByRole('combobox').press('Tab')
    await saved(row)

    await addCustom(page, 'Звание', 'Выбор или своё', { options: 'Рядовой\nСержант' })
    row = fieldRow(page, 'Звание')
    await row.getByRole('combobox', { name: 'Звание' }).click()
    await page.getByRole('option', { name: 'Сержант' }).click()
    await saved(row)

    await addCustom(page, shared, 'Текст', { toAll: true })
    row = fieldRow(page, shared)
    await row.getByRole('textbox').fill('для всех')
    await row.getByRole('textbox').press('Tab')
    await saved(row)

    await page.reload()
    await editMode(page, false)
    for (const [f, v] of [['Рост', '182'], ['Крещение', '1951'], ['Ветеран', 'Нет'], ['Группа крови', 'Вторая'], ['Звание', 'Сержант'], [shared, 'для всех']] as const) {
      await expect(fieldRow(page, f).locator('.values'), f).toContainText(v)
    }
  })

  await test.step('своё поле: ссылка на запись и изображение', async () => {
    const org = await createRecord(page, 'Организация', 'Организация', 'Название', `Завод ${stamp}`)
    await page.goto(person)
    await editMode(page, true)
    await addCustom(page, 'Работа', 'Ссылка на запись')
    const row = fieldRow(page, 'Работа')
    await row.getByRole('button', { name: 'Выбрать запись' }).click()
    await row.getByRole('searchbox', { name: 'Выбрать запись' }).fill(`Завод ${stamp}`)
    await row.getByRole('button', { name: new RegExp(`Завод ${stamp}`) }).click()
    await row.getByRole('button', { name: 'Сохранить' }).click()
    await expect(row.locator('.values')).toContainText(`Завод ${stamp}`)

    await addCustom(page, 'Скан', 'Изображение')
    await page.getByRole('button', { name: 'Загрузить изображение: Скан' }).click()
    await page.getByRole('dialog').locator('input[type=file]').setInputFiles({ name: 'scan.png', mimeType: 'image/png', buffer: PNG })
    await page.getByRole('dialog', { name: 'Обрезка' }).getByRole('button', { name: 'Без обрезки' }).click()
    await expect(fieldRow(page, 'Скан').locator('img')).toBeVisible()
    await page.reload()
    await editMode(page, false)
    await expect(fieldRow(page, 'Работа').locator('.values')).toContainText(`Завод ${stamp}`)
    await expect(fieldRow(page, 'Скан').locator('img')).toBeVisible()
    void org
  })

  await test.step('организация: «Форма» — вариант из списка и своё; контакт', async () => {
    await createRecord(page, 'Организация', 'Организация', 'Название', `Ромашка ${stamp}`)
    let row = fieldRow(page, 'Форма')
    await row.getByRole('combobox', { name: 'Форма' }).click()
    await page.getByRole('option', { name: 'ООО', exact: true }).click()
    await saved(row)
    await reveal(page, 'Директор')
    row = fieldRow(page, 'Директор')
    await row.getByRole('textbox').fill('Иванов')
    await row.getByRole('textbox').press('Tab')
    await saved(row)
    await page.reload()
    await editMode(page, false)
    await expect(page.locator('h1')).toContainText(`ООО «Ромашка ${stamp}»`)
    await expect(fieldRow(page, 'Директор').locator('.values')).toContainText('Иванов')

    // своё значение: ввод и закрытие списка кликом мимо
    await editMode(page, true)
    row = fieldRow(page, 'Форма')
    await row.getByRole('combobox', { name: 'Форма' }).fill('ЖСК')
    await page.locator('h1').click()
    await replaceIfAsked(page, 'Форма', row)
    await saved(row)
    await persisted(page, 'Форма', 'ЖСК')
  })

  await test.step('настройки дашборда: название; облака — каждый шаг сразу, быстрая форма видит без перезагрузки', async () => {
    await page.goto(person)
    const settings = page.getByRole('dialog', { name: 'Настройки дашборда' })
    const openSettings = async () => {
      await page.getByRole('button', { name: 'Настройки дашборда' }).click()
      await expect(settings).toBeVisible()
    }
    await openSettings()
    await settings.getByRole('textbox', { name: 'Название дашборда' }).fill(`Дашборд ${stamp}`)
    await settings.getByRole('button', { name: 'Сохранить', exact: true }).click()
    await settings.getByRole('tab', { name: 'Шаблоны' }).click()
    const island = settings.getByTestId('quick-kin.core.person')
    const inForm = island.getByRole('list', { name: 'В быстрой форме', exact: true })
    const notInForm = island.getByRole('list', { name: 'Не в быстрой форме', exact: true })
    await expect(island.getByRole('button', { name: /^Сохранить/ })).toHaveCount(0)
    // с набора пака (повторный прогон на той же установке)
    await island.getByRole('button', { name: 'По умолчанию' }).click()
    await expect(island).toHaveAttribute('data-state', 'saved')
    await expect(inForm.locator('.pill')).toHaveText(['Имя', 'Фамилия', 'Пол', 'Примечание', 'Дата рождения'])
    // каждый шаг — сразу; быстрые клики подряд не теряются
    await notInForm.getByRole('button', { name: 'Отчество' }).click()
    await notInForm.getByRole('button', { name: 'Ник' }).click()
    await inForm.getByRole('button', { name: 'Примечание' }).click()
    await expect(island).toHaveAttribute('data-state', 'saved')
    // порядок — Alt + ←: «Ник» перед «Отчеством»
    await inForm.getByRole('button', { name: 'Ник' }).press('Alt+ArrowLeft')
    await expect(island).toHaveAttribute('data-state', 'saved')
    const expected = ['Имя', 'Фамилия', 'Пол', 'Дата рождения', 'Ник', 'Отчество']
    await expect(inForm.locator('.pill')).toHaveText(expected)
    await page.keyboard.press('Escape')
    await expect(settings).toBeHidden()

    // быстрая форма уже с новыми полями — без перезагрузки
    await page.getByRole('button', { name: 'Быстро добавить' }).click()
    await page.getByRole('button', { name: 'Добавить мать', exact: true }).click()
    const form = page.getByRole('dialog', { name: 'Добавить мать' })
    await expect(form.getByRole('textbox', { name: 'Отчество' })).toBeVisible()
    await expect(form.getByRole('textbox', { name: 'Примечание' })).toHaveCount(0)
    await form.getByRole('textbox', { name: 'Имя' }).fill('Анна')
    await form.getByRole('textbox', { name: 'Отчество' }).fill('Петровна')
    await form.getByRole('button', { name: 'Сохранить' }).click()
    await expect(form).toBeHidden()

    await page.reload()
    await expect(page.locator('.relation').filter({ hasText: 'Анна' })).toBeVisible()
    await expect(page.getByRole('combobox', { name: 'Дашборд' }).locator('option:checked')).toHaveText(`Дашборд ${stamp}`)
    await openSettings()
    await settings.getByRole('tab', { name: 'Шаблоны' }).click()
    await expect(inForm.locator('.pill')).toHaveText(expected)
    // «По умолчанию» — тоже сразу
    await island.getByRole('button', { name: 'По умолчанию' }).click()
    await expect(island).toHaveAttribute('data-state', 'saved')
    await page.reload()
    await openSettings()
    await settings.getByRole('tab', { name: 'Шаблоны' }).click()
    await expect(inForm.locator('.pill')).toHaveText(['Имя', 'Фамилия', 'Пол', 'Примечание', 'Дата рождения'])
    await page.keyboard.press('Escape')
  })

  await test.step('группы: создание, автодобавление по фамилии; выделение — добавить и убрать', async () => {
    const group = `Группа ${stamp}`
    const auto = `Авто ${stamp}`
    const groupsPage = async () => {
      await page.getByRole('link', { name: 'Группы' }).click()
      await expect(page.getByLabel('Название группы')).toBeVisible()
    }
    const groupRow = (name: string) => page.locator('main li').filter({ hasText: name }).first()
    await groupsPage()
    await page.getByLabel('Название группы').fill(group)
    await page.getByRole('button', { name: 'Создать' }).click()
    await expect(groupRow(group)).toBeVisible()
    await page.getByLabel('Название группы').fill(auto)
    await page.getByRole('button', { name: 'Создать' }).click()
    await page.getByRole('button', { name: `Изменить: ${auto}` }).click()
    await page.getByRole('checkbox', { name: 'Автоматически добавлять по фамилии' }).check()
    await page.getByRole('textbox', { name: /^Фамилии/ }).fill(surname)
    await page.locator('.kin-popover').getByRole('button', { name: 'Сохранить' }).click()
    // Пётр и его мать Анна (фамилия предзаполнена быстрой формой)
    await expect(groupRow(auto)).toContainText('записей: 2')
    await page.reload()
    await expect(groupRow(group)).toBeVisible()
    await expect(groupRow(auto)).toContainText('записей: 2')
    await page.getByRole('button', { name: `Изменить: ${auto}` }).click()
    await expect(page.getByRole('checkbox', { name: 'Автоматически добавлять по фамилии' })).toBeChecked()
    await expect(page.getByRole('textbox', { name: /^Фамилии/ })).toHaveValue(new RegExp(surname, 'i'))
    await page.keyboard.press('Escape')

    // выделение в «Списках»: добавить в группу, затем убрать
    const sel = page.getByTestId('tree-selection')
    const pick = async () => {
      await page.getByRole('link', { name: 'Списки', exact: true }).click()
      await page.getByRole('tab', { name: 'Человек' }).click()
      await page.getByRole('checkbox', { name: new RegExp(`^Выделить: Пётр.*${surname}`) }).check()
      await expect(sel).toContainText('Выделено: 1')
      await sel.getByRole('combobox').selectOption({ label: group })
    }
    await pick()
    await sel.getByRole('button', { name: 'Добавить в группу' }).click()
    await expect(sel).toContainText('Готово: 1')
    await page.reload()
    await groupsPage()
    await expect(groupRow(group)).toContainText('записей: 1')
    await pick()
    await sel.getByRole('button', { name: 'Убрать из группы' }).click()
    await expect(sel).toContainText('Готово: 1')
    await page.reload()
    await groupsPage()
    await expect(groupRow(group)).toContainText('записей: 0')
  })

  await test.step('режимы карточки: выбор сохраняется; «По месту» — текст и «выбор или своё» из списка', async () => {
    const mode = (name: RegExp) => page.getByRole('radio', { name })
    const setMode = async (name: RegExp) => {
      await page.getByRole('link', { name: LOGIN }).click()
      await mode(name).check()
      await expect(mode(name)).toBeChecked()
      await page.reload()
      await expect(mode(name)).toBeChecked()
    }
    await setMode(/^По месту/)

    await page.goto(person)
    await page.getByRole('button', { name: 'Изменить: Отчество' }).click()
    let row = fieldRow(page, 'Отчество')
    await row.getByRole('textbox', { name: 'Отчество' }).fill('Иванович')
    await row.getByRole('textbox', { name: 'Отчество' }).press('Enter')
    await replaceIfAsked(page, 'Отчество', row)
    await expect(row.getByRole('textbox')).toHaveCount(0)
    await page.reload()
    await expect(fieldRow(page, 'Отчество').locator('.values')).toContainText('Иванович')

    await createRecord(page, 'Организация', 'Организация', 'Название', `Лютик ${stamp}`)
    await page.reload() // черновик открыт в правке; «По месту» — после перезагрузки
    row = fieldRow(page, 'Форма')
    await page.getByRole('button', { name: 'Изменить: Форма' }).click()
    await row.getByRole('combobox', { name: 'Форма' }).click()
    await page.getByRole('option', { name: 'ИП', exact: true }).click()
    await expect(row.locator('.values')).toContainText('ИП')
    await page.reload()
    await expect(fieldRow(page, 'Форма').locator('.values')).toContainText('ИП')

    await setMode(/^Форма/)
    await page.goto(person)
    await expect(page.getByRole('button', { name: 'Готово' })).toBeVisible()
    await setMode(/^Чтение/)
  })
})

/** Поле с историей и значением спрашивает «заменить или добавить» (F-80) — заменить. */
async function replaceIfAsked(page: Page, field: string, row: Locator) {
  const ask = page.getByRole('dialog', { name: `Изменить «${field}»` })
  await expect(ask.or(row.locator('.cell[data-state="saved"]')).or(row.locator('.values')).first()).toBeVisible()
  if (await ask.isVisible()) await ask.getByRole('button', { name: 'Заменить текущее' }).click()
}
