// Сквозной сценарий 0.1 (T27) на свежей установке: настройка по токену →
// дашборд → черновик новой записи (F-88) → человек в режиме редактирования
// (F-85, F-86) с частичной датой и историей фамилий (F-80) → (+) отец с
// предзаполненной фамилией → (+) муж: «Пара» на этапе «Брак» → свадьба с
// двумя участниками и новым свидетелем прямо в диалоге (F-87) → «Связи»
// (F-89) → фото с обрезками по местам (F-79) → своё поле → группы и фильтр →
// корзина, восстановление, очистка → журнал. Тексты — интерфейс ru.
import { expect, test, type Locator, type Page } from '@playwright/test'

const TOKEN = process.env.KIN_E2E_TOKEN ?? ''
const LOGIN = 'owner'
const PASSWORD = 'e2e-password-0.1'
const NEW_PASSWORD = 'e2e-password-0.1-new'

/** PNG 64×48: обрезка — квадрат внутри прямоугольника */
const PNG = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAEAAAAAwCAIAAAAuKetIAAAAQ0lEQVR42u3PQQkAAAgEsAtmMCMaywp+hcEKLNP1WgQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQErhYYG9EAD2sj5gAAAABJRU5ErkJggg==',
  'base64',
)

function fieldRow(page: Page, name: string): Locator {
  return page.locator('.field-row').filter({ has: page.locator('.label', { hasText: new RegExp(`^${name}$`) }) })
}

/** Поле в режиме редактирования (F-85): ввод → уход из поля сохраняет (F-86). */
async function setText(page: Page, name: string, value: string) {
  const input = fieldRow(page, name).getByRole('textbox', { name, exact: true })
  await input.fill(value)
  await input.press('Tab')
  await expect(fieldRow(page, name).locator('.cell')).toHaveAttribute('data-state', 'saved')
  await expect(input).toHaveValue(value)
}

/** Режим карточки (F-85). */
async function editMode(page: Page, on: boolean) {
  await page.getByRole('button', { name: on ? 'Редактировать' : 'Готово' }).click()
  await expect(page.getByRole('button', { name: on ? 'Готово' : 'Редактировать' })).toBeVisible()
}

async function partialDate(scope: Locator, d: { d?: string; m?: string; y?: string }) {
  if (d.d) await scope.getByRole('textbox', { name: 'День' }).fill(d.d)
  if (d.m) await scope.getByRole('textbox', { name: 'Месяц' }).fill(d.m)
  if (d.y) await scope.getByRole('textbox', { name: 'Год' }).fill(d.y)
}

/** Быстрое добавление (+): пункт меню → однострочная форма. */
async function quickAdd(page: Page, item: string): Promise<Locator> {
  await page.getByRole('button', { name: 'Быстро добавить' }).click()
  await page.getByRole('button', { name: item, exact: true }).click()
  const dialog = page.getByRole('dialog', { name: item })
  await expect(dialog).toBeVisible()
  return dialog
}

test('основной сценарий 0.1', async ({ page }) => {
  expect(TOKEN, 'KIN_E2E_TOKEN — токен из лога (e2e/run.sh)').not.toBe('')
  let liza = ''

  await test.step('настройка по токену → дашборд', async () => {
    await page.goto('/')
    await expect(page).toHaveURL(/\/setup$/)
    await page.getByLabel('Токен первого запуска').fill(TOKEN)
    await page.getByLabel('Логин').fill(LOGIN)
    await page.getByLabel('Пароль', { exact: true }).fill(PASSWORD)
    await page.getByLabel('Пароль ещё раз').fill(PASSWORD)
    await page.getByRole('button', { name: 'Создать владельца' }).click()
    await expect(page).toHaveURL(/\/d\/[0-9a-f-]+/)
    await expect(page.getByText('Записей нет')).toBeVisible()
  })

  await test.step('черновик: ушли, ничего не заполнив, — записи нет (F-88)', async () => {
    await page.getByRole('tab', { name: 'Человек' }).click()
    await page.getByRole('button', { name: 'Создать' }).click()
    await expect(page).toHaveURL(/\/r\/draft-/)
    await expect(page.getByText('Новая запись появится, когда вы заполните первое поле')).toBeVisible()
    await page.getByRole('link', { name: 'Назад' }).click()
    await expect(page.getByText('Записей нет')).toBeVisible()
  })

  await test.step('человек: имя, история фамилий, пол, дата рождения без года', async () => {
    await page.getByRole('button', { name: 'Создать' }).click()
    await expect(page).toHaveURL(/\/r\/draft-/)
    // первое заполненное поле создаёт запись (F-88), карточка остаётся в правке
    const name = fieldRow(page, 'Имя').getByRole('textbox', { name: 'Имя', exact: true })
    await name.fill('Лиза')
    await name.press('Tab')
    await expect(page).toHaveURL(/\/r\/[0-9a-f]{8}-[0-9a-f-]+$/)
    liza = page.url()
    await expect(page.getByRole('button', { name: 'Готово' })).toBeVisible()
    await setText(page, 'Фамилия', 'Иванова')

    // Esc в фокусе — отмена изменений поля (F-86)
    const surname = fieldRow(page, 'Фамилия').getByRole('textbox', { name: 'Фамилия', exact: true })
    await surname.fill('Опечатка')
    await expect(fieldRow(page, 'Фамилия').locator('.cell')).toHaveAttribute('data-state', 'dirty')
    await surname.press('Escape')
    await expect(surname).toHaveValue('Иванова')

    // у поля есть история и значение — спросить (F-80): новая фамилия сверху, девичья остаётся ниже
    await surname.fill('Василькова')
    await surname.press('Tab')
    const ask = page.getByRole('dialog', { name: 'Изменить «Фамилия»' })
    await ask.getByRole('button', { name: 'Добавить новое в историю' }).click()
    await expect(ask).toBeHidden()
    await expect(page.locator('h1')).toContainText('Лиза Василькова')
    await page.getByRole('button', { name: 'История значений: Фамилия' }).click()
    await expect(page.locator('.history .item .text')).toHaveText(['Василькова', 'Иванова'])
    await page.keyboard.press('Escape')

    const gender = fieldRow(page, 'Пол').getByRole('combobox')
    await gender.selectOption({ label: 'Женский' })
    await gender.press('Tab')
    await expect(fieldRow(page, 'Пол').locator('.cell')).toHaveAttribute('data-state', 'saved')

    const born = page.locator('.sys-row').filter({ hasText: 'Дата рождения' })
    await partialDate(born, { d: '12', m: '3' })
    await born.getByRole('textbox', { name: 'Год' }).focus()
    await born.getByRole('textbox', { name: 'Год' }).press('Tab')
    await expect(born.locator('.cell')).toHaveAttribute('data-state', 'saved')

    // «Умер» (F-126) — в шапке за «Добавить дату»; «да/нет» — сегменты в один клик, повторный клик очищает
    await page.getByRole('button', { name: 'Добавить дату' }).click()
    await page.getByRole('button', { name: 'Умер' }).click()
    const dead = page.locator('.basics').locator('.field-row').filter({ hasText: 'Умер' })
    await dead.getByRole('button', { name: 'Да' }).click()
    await expect(dead.locator('.cell')).toHaveAttribute('data-state', 'saved')
    await expect(dead.getByRole('button', { name: 'Да' })).toHaveAttribute('aria-pressed', 'true')
    await dead.getByRole('button', { name: 'Да' }).click()
    await expect(dead.getByRole('button', { name: 'Да' })).toHaveAttribute('aria-pressed', 'false')
    await expect(dead.getByRole('button', { name: 'Нет' })).toHaveAttribute('aria-pressed', 'false')

    await editMode(page, false)
    await expect(born.locator('.value')).toHaveText('12 марта')
    await expect(fieldRow(page, 'Пол').locator('.values')).toContainText('Женский')
    await expect(fieldRow(page, 'Фамилия').locator('.values')).toHaveText('Василькова')
  })

  await test.step('(+) «Добавить отца»: фамилия предзаполнена девичьей; пункт исчезает', async () => {
    const form = await quickAdd(page, 'Добавить отца')
    await expect(form.getByRole('textbox', { name: 'Фамилия' })).toHaveValue('Иванова')
    await form.getByRole('textbox', { name: 'Имя' }).fill('Сергей')
    await form.getByRole('button', { name: 'Сохранить' }).click()
    await expect(form).toBeHidden()
    const rel = page.locator('.relation').filter({ hasText: 'Сергей Иванов' })
    await expect(rel).toContainText('Папа: Сергей Иванова (через Родитель — ребёнок)') // фамилия не склоняется (F-74)
    await page.getByRole('button', { name: 'Быстро добавить' }).click()
    await expect(page.getByRole('button', { name: 'Добавить мать', exact: true })).toBeEnabled()
    await expect(page.getByRole('button', { name: 'Добавить отца', exact: true })).toHaveCount(0)
    await page.keyboard.press('Escape')
  })

  await test.step('(+) «Добавить мужа»: отношение «Пара» на этапе «Брак»', async () => {
    const form = await quickAdd(page, 'Добавить мужа')
    await form.getByRole('textbox', { name: 'Имя' }).fill('Иван')
    await form.getByRole('textbox', { name: 'Фамилия' }).fill('Васильков')
    await form.getByRole('button', { name: 'Сохранить' }).click()
    await expect(form).toBeHidden()
    const rel = page.locator('.relation').filter({ hasText: 'Иван Васильков' })
    await expect(rel).toContainText('Муж: Иван Васильков (через Брак)') // своя роль не пишется (F-89)
    await expect(rel).not.toContainText('Жена')
  })

  await test.step('свадьба: два участника, без второго не создать', async () => {
    await page.getByRole('button', { name: 'Связь' }).click()
    await page.getByRole('button', { name: 'Свадьба', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Новая запись: Свадьба' })
    await expect(dialog.locator('.picked')).toContainText(['Лиза Василькова'])
    await expect(dialog.getByRole('button', { name: 'Создать' })).toBeDisabled()
    await dialog.locator('.role').first().getByRole('button', { name: 'Добавить участника' }).click()
    await dialog.getByRole('searchbox', { name: 'Выбрать запись' }).fill('Иван')
    await dialog.getByRole('button', { name: /^Иван Васильков/ }).click()
    // свидетельницу — новой записью прямо в диалоге (F-87)
    const witness = dialog.locator('.role').filter({ hasText: 'Свидетель' })
    await witness.getByRole('button', { name: 'Новый' }).click()
    await witness.getByRole('textbox', { name: 'Имя' }).fill('Ольга')
    await witness.getByRole('button', { name: 'Добавить', exact: true }).click()
    await expect(witness.locator('.picked')).toContainText('Ольга')
    await partialDate(dialog, { y: '2021' })
    await dialog.getByRole('button', { name: 'Создать' }).click()
    await expect(dialog).toBeHidden()
    // создание из карточки оставляет на ней, созданное — в «Связях» (F-101)
    await expect(page).toHaveURL(liza)
    await expect(page.locator('h1')).toContainText('Лиза Василькова')
    // событие — прямая ссылка, без людей через него (F-93)
    const wedding = page.locator('.relation.link').filter({ hasText: 'Свадьба' }).first()
    await expect(wedding).toBeVisible()
    await expect(wedding).not.toContainText('Жених')
    // по ссылке в связь и «назад» — на исходную карточку (F-101)
    await wedding.getByRole('link', { name: /Свадьба/ }).first().click()
    await expect(page.locator('h1')).toContainText('Свадьба')
    await expect(page.locator('h1')).toContainText('2021')
    await expect(page.locator('main')).toContainText('Ольга')
    await page.getByRole('link', { name: 'Назад' }).click()
    await expect(page).toHaveURL(liza)
    await expect(page.locator('h1')).toContainText('Лиза Василькова')
  })

  await test.step('фото: диалог загрузки (F-107) — перетаскивание и Ctrl+V, обрезки (F-79), превью', async () => {
    await editMode(page, true)
    const pick = page.getByRole('dialog', { name: 'Загрузить изображение' })
    const crop = page.getByRole('dialog', { name: 'Обрезка' })
    const upload = fieldRow(page, 'Фото').getByRole('button', { name: 'Загрузить изображение: Фото' })
    // перетаскивание: не картинка — ошибка в диалоге, картинка — к обрезке
    const dropFile = async (name: string, type: string, b64: string) => {
      const dt = await page.evaluateHandle(([n, t, data]) => {
        const d = new DataTransfer()
        d.items.add(new File([Uint8Array.from(atob(data!), c => c.charCodeAt(0))], n!, { type: t }))
        return d
      }, [name, type, b64])
      await pick.locator('.drop').dispatchEvent('drop', { dataTransfer: dt })
    }
    await upload.click()
    await expect(pick).toBeVisible()
    await dropFile('notes.txt', 'text/plain', Buffer.from('не картинка').toString('base64'))
    await expect(pick.getByRole('alert')).toContainText('Это не изображение')
    await dropFile('liza.png', 'image/png', PNG.toString('base64'))
    await expect(pick).toBeHidden()
    await expect(crop.locator('.frame')).toBeVisible()
    await crop.getByRole('button', { name: 'Отмена' }).click()
    await expect(crop).toBeHidden()
    // Ctrl+V: картинка из буфера, пока диалог открыт
    await upload.click()
    await expect(pick).toBeVisible()
    await page.evaluate(data => {
      const d = new DataTransfer()
      d.items.add(new File([Uint8Array.from(atob(data), c => c.charCodeAt(0))], 'clip.png', { type: 'image/png' }))
      document.activeElement!.dispatchEvent(new ClipboardEvent('paste', { clipboardData: d, bubbles: true, cancelable: true }))
    }, PNG.toString('base64'))
    await expect(pick).toBeHidden()
    await expect(crop.locator('.frame')).toBeVisible()
    await expect(crop.getByRole('tab')).toHaveText(['Карточка', 'Дерево', 'Сеть'])
    await crop.getByRole('tab', { name: 'Сеть' }).click()
    await crop.getByRole('radio', { name: 'Круг' }).click()
    await crop.getByRole('button', { name: 'Сохранить' }).click()
    await expect(crop).toBeHidden()
    // карточка — своя обрезка (портрет по умолчанию)
    const avatar = page.locator('.head img')
    await expect(avatar).toHaveAttribute('src', /\/preview\?.*shape=rect/)
    const res = await page.request.get((await avatar.getAttribute('src'))!)
    expect(res.status()).toBe(200)
    expect(res.headers()['content-type']).toMatch(/^image\/jpeg/)
    // сеть и списки — кругом
    const round = (await avatar.getAttribute('src'))!.replace(/shape=rect/, 'shape=circle').replace(/&x=.*$/, '')
    const circle = await page.request.get(round)
    expect(circle.headers()['content-type']).toBe('image/png')
  })

  await test.step('своё поле: описание и значение с историей (F-75)', async () => {
    await page.getByRole('button', { name: 'Своё поле' }).click()
    const pop = page.locator('.custom-form')
    await pop.getByLabel('Название').fill('Воинская часть')
    await pop.getByLabel('С историей значений').check()
    await pop.getByRole('button', { name: 'Сохранить' }).click()
    await expect(page.getByRole('heading', { name: 'Свои поля' })).toBeVisible()
    const input = fieldRow(page, 'Воинская часть').getByRole('textbox')
    await input.fill('в/ч 1234')
    await input.press('Tab')
    await expect(fieldRow(page, 'Воинская часть').locator('.cell')).toHaveAttribute('data-state', 'saved')
    await page.reload()
    await expect(fieldRow(page, 'Воинская часть').locator('.values')).toContainText('в/ч 1234')
  })

  await test.step('своё поле «выбор или своё»: своё значение и вариант по названию (F-100)', async () => {
    await editMode(page, true)
    await page.getByRole('button', { name: 'Своё поле' }).click()
    const pop = page.locator('.custom-form')
    await pop.getByLabel('Название').fill('Настроение')
    await pop.getByLabel('Тип').selectOption({ label: 'Выбор или своё' })
    await pop.getByLabel('Варианты, по одному в строке').fill('Весело\nГрустно\nОчень длинный вариант настроения, который не помещается в ширину поля ввода')
    await pop.getByRole('button', { name: 'Сохранить' }).click()
    const row = () => fieldRow(page, 'Настроение')
    const box = row().getByRole('combobox', { name: 'Настроение' })
    // список — ровно по ширине поля ввода, длинный вариант переносится
    await row().getByRole('button', { name: 'Варианты' }).click()
    const list = page.locator('.combo-list')
    await expect(list).toBeVisible()
    expect(Math.abs((await list.boundingBox())!.width - (await box.boundingBox())!.width)).toBeLessThan(1)
    const long = list.getByRole('option', { name: /Очень длинный/ })
    const lb = (await long.boundingBox())!
    expect(lb.height).toBeGreaterThan(1.5 * (await list.getByRole('option', { name: 'Весело' }).boundingBox())!.height)
    expect(await long.evaluate((el) => el.scrollWidth <= el.clientWidth)).toBe(true)
    await row().getByRole('button', { name: 'Варианты' }).click()
    await expect(list).toBeHidden()
    await box.fill('Задумчиво')
    await box.press('Tab')
    await expect(row().locator('.cell')).toHaveAttribute('data-state', 'saved')
    await page.reload()
    await expect(row().locator('.values')).toContainText('Задумчиво')
    // своё, совпавшее с названием варианта, хранится ключом варианта
    await editMode(page, true)
    await row().getByRole('combobox', { name: 'Настроение' }).fill('весело')
    await row().getByRole('combobox', { name: 'Настроение' }).press('Tab')
    await expect(row().locator('.cell')).toHaveAttribute('data-state', 'saved')
    await page.reload()
    await expect(row().locator('.values')).toContainText('Весело')
  })

  await test.step('группы: «Ивановы» входят в «Нашу семью», фильтр с вложенными', async () => {
    // прямой заход по адресу: «назад» — в список (F-101)
    await page.goto(liza)
    await page.getByRole('link', { name: 'Назад' }).click()
    await expect(page.getByRole('link', { name: 'Группы' })).toBeVisible()
    await page.getByRole('link', { name: 'Группы' }).click()
    await page.getByLabel('Название группы').fill('Наша семья')
    await page.getByRole('button', { name: 'Создать' }).click()
    await expect(page.locator('main')).toContainText('Наша семья')
    await page.getByLabel('Название группы').fill('Ивановы')
    // «Входит в» — мультивыбор (F-105)
    await page.getByRole('button', { name: 'Входит в' }).click()
    await page.getByRole('checkbox', { name: 'Наша семья' }).check()
    await page.keyboard.press('Escape')
    await page.getByRole('button', { name: 'Создать' }).click()
    await expect(page.locator('main')).toContainText('Ивановы')
    await expect(page.locator('main')).toContainText('Входит в: 1')
    await expect(page.locator('main')).toContainText('Включает в себя: 1')

    await page.goto(liza)
    await page.locator('.relation').filter({ hasText: 'Сергей Иванов' }).getByRole('link', { name: 'Сергей Иванов' }).click()
    await expect(page.locator('h1')).toContainText('Сергей Иванов')
    await editMode(page, true)
    await page.getByRole('button', { name: 'Изменить: Группы' }).click()
    await page.getByRole('button', { name: 'Группа' }).click()
    await page.getByRole('checkbox', { name: 'Ивановы' }).check()
    await page.locator('form').filter({ has: page.locator('.group-picker') }).getByRole('button', { name: 'Сохранить' }).click()
    await expect(page.locator('.groups-row .chip')).toHaveText(['Ивановы'])

    // пришёл с карточки Лизы — «назад» на неё, оттуда (прямой заход) — в список
    await page.getByRole('link', { name: 'Назад' }).click()
    await expect(page.locator('h1')).toContainText('Лиза Василькова')
    await page.getByRole('link', { name: 'Назад' }).click()
    await page.getByRole('combobox', { name: 'Группа' }).selectOption({ label: 'Наша семья' })
    const list = page.locator('.record-list')
    await expect(list).toContainText('Сергей Иванов')
    await expect(list).not.toContainText('Лиза')
  })

  await test.step('группа по фамилии (F-122) и выделение в списках (F-131)', async () => {
    await page.goto(liza)
    await page.getByRole('link', { name: 'Назад' }).click()
    await page.getByRole('link', { name: 'Группы' }).click()
    await page.getByLabel('Название группы').fill('По фамилии')
    await page.getByRole('button', { name: 'Создать' }).click()
    await page.getByRole('button', { name: 'Изменить: По фамилии' }).click()
    await page.getByRole('checkbox', { name: 'Автоматически добавлять по фамилии' }).check()
    await page.getByRole('textbox', { name: /^Фамилии/ }).fill('иванова')
    await page.locator('.kin-popover').getByRole('button', { name: 'Сохранить' }).click()
    await expect(page.locator('main')).toContainText('записей: 2')
    await page.getByRole('link', { name: 'Списки', exact: true }).click()
    // галочки у строк и панель справа
    await page.getByRole('checkbox', { name: 'Выделить: Лиза Василькова' }).check()
    const sel = page.getByTestId('tree-selection')
    await expect(sel).toContainText('Выделено: 1')
    await sel.getByRole('combobox').selectOption({ label: 'Новая группа' })
    await sel.getByRole('textbox', { name: 'Название группы' }).fill('Из списка')
    await sel.getByRole('button', { name: 'Добавить в группу' }).click()
    await expect(sel).toContainText('Готово: 1')
    await sel.getByRole('button', { name: 'Снять выделение' }).click()
    await expect(sel).toBeHidden()
  })

  await test.step('режим карточек «Форма» (F-108): по ссылке и «назад» — сразу правка', async () => {
    const mode = (name: RegExp) => page.getByRole('radio', { name })
    await page.getByRole('link', { name: LOGIN }).click()
    await mode(/^Форма/).check()
    await expect(mode(/^Форма/)).toBeChecked()
    await page.goto(liza)
    await expect(page.getByRole('button', { name: 'Готово' })).toBeVisible()
    await page.locator('.relation').filter({ hasText: 'Сергей Иванов' }).getByRole('link', { name: 'Сергей Иванов' }).click()
    await expect(page.locator('h1')).toContainText('Сергей Иванов')
    await expect(page.getByRole('button', { name: 'Готово' })).toBeVisible()
    await expect(fieldRow(page, 'Имя').getByRole('textbox', { name: 'Имя', exact: true })).toBeVisible()
    await page.getByRole('link', { name: 'Назад' }).click()
    await expect(page.locator('h1')).toContainText('Лиза Василькова')
    await expect(page.getByRole('button', { name: 'Готово' })).toBeVisible()
    // обратно в «Чтение»: карточка открывается для чтения
    await page.getByRole('link', { name: LOGIN }).click()
    await mode(/^Чтение/).check()
    await expect(mode(/^Чтение/)).toBeChecked()
    await page.goto(liza)
    await expect(page.getByRole('button', { name: 'Редактировать' })).toBeVisible()
  })

  await test.step('дерево (F-110…F-119): узлы, карточка по клику, (+) мать, «Скрыть умерших»', async () => {
    await page.goto(liza.replace(/\/r\/.*$/, '?v=tree'))
    const node = (name: string) => page.getByTestId('tree-person').filter({ hasText: name })
    await expect(node('Лиза')).toBeVisible()
    await expect(node('Сергей')).toBeVisible()
    await expect(node('Иван Васильков')).toBeVisible()
    await expect(page.locator('.tn-union', { hasText: 'Брак' })).toBeVisible()
    // клик по узлу — полная карточка поверх дерева (F-113)
    await node('Лиза').locator('.tn-name').click()
    const card = page.getByRole('dialog')
    await expect(card.locator('h1')).toContainText('Лиза Василькова')
    // низкий экран: окно карточки без своей прокрутки, до последнего поля доходит оверлей
    await page.setViewportSize({ width: 390, height: 480 })
    await card.locator('.mode').click()
    const overlay = page.locator('.tree-overlay')
    await overlay.evaluate((el) => { el.scrollTop = el.scrollHeight })
    expect(await overlay.evaluate((el) => el.scrollTop)).toBeGreaterThan(0)
    const box = (await card.boundingBox())!
    expect(box.y + box.height).toBeLessThanOrEqual(480)
    await page.setViewportSize({ width: 1280, height: 720 })
    await page.keyboard.press('Escape')
    await expect(card).toBeHidden()
    await expect(page).toHaveURL(/\?v=tree/)
    // (+) на узле — настоящий quick_add, после создания остаёмся в дереве (F-101)
    await node('Лиза').hover()
    await node('Лиза').getByRole('button', { name: 'Добавить' }).click()
    await page.getByRole('menuitem', { name: 'Добавить мать' }).click()
    const form = page.getByRole('dialog', { name: 'Добавить мать' })
    await form.getByRole('textbox', { name: 'Имя' }).fill('Марина')
    await form.getByRole('button', { name: 'Сохранить' }).click()
    await expect(form).toBeHidden()
    await expect(node('Марина')).toBeVisible()
    await expect(page).toHaveURL(/\?v=tree/)
    // «Существующий» (F-130): поиск по ФИО, связь с выбранным — новой записи нет
    await node('Марина').hover()
    await node('Марина').getByRole('button', { name: 'Добавить' }).click()
    await page.getByRole('menuitem', { name: 'Добавить ребёнка' }).click()
    const kid = page.getByRole('dialog', { name: 'Добавить ребёнка' })
    await kid.getByTestId('mode-existing').click()
    await kid.getByTestId('existing-search').fill('Иван Вас')
    await kid.getByTestId('candidate').filter({ hasText: 'Иван Васильков' }).click()
    await kid.getByRole('button', { name: 'Сохранить' }).click()
    await expect(kid).toBeHidden()
    await expect(page.getByTestId('tree-person').filter({ hasText: 'Иван Васильков' })).toHaveCount(1)
    await node('Иван Васильков').locator('.tn-name').click()
    await expect(page.getByRole('dialog').locator('.relations')).toContainText('Марина')
    await page.keyboard.press('Escape')
    // значок отсылки на основной карточке — перелёт к отсылке (F-117)
    await page.getByRole('button', { name: 'Вписать всё' }).click()
    await page.waitForTimeout(600)
    const badge = page.getByTestId('tree-ref-badge').first()
    await expect(badge).toBeVisible()
    await expect(badge).toHaveAttribute('title', /^Перейти к отсылке/)
    const refId = (await badge.getAttribute('data-ref'))!
    await badge.click()
    const refNode = page.locator(`.vue-flow__node[data-id="${refId}"]`)
    await expect(refNode.locator('.tn-ref')).toHaveClass(/is-hl/)
    await expect.poll(async () => {
      const c = (await page.locator('.tree-canvas').boundingBox())!
      const n = (await refNode.boundingBox())!
      return Math.abs(n.x + n.width / 2 - (c.x + c.width / 2)) < 20 && Math.abs(n.y + n.height / 2 - (c.y + c.height / 2)) < 20
    }).toBe(true)
    // двойной клик по линии — карточка отношения или меню отношений (F-127)
    const edge = page.locator('.vue-flow__edge[data-id^="b:"]').first()
    await edge.locator('path').first().dispatchEvent('dblclick', { bubbles: true, clientX: 300, clientY: 300 })
    await expect(page.getByRole('dialog').or(page.getByTestId('tree-edge-menu'))).toBeVisible()
    if (await page.getByTestId('tree-edge-menu').isVisible()) await page.getByTestId('tree-edge-menu').getByRole('menuitem').first().click()
    await expect(page.getByRole('dialog')).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(page.getByRole('dialog')).toBeHidden()
    // выделение Ctrl + клик и панель групп (F-131)
    await page.getByRole('button', { name: 'Вписать всё' }).click()
    await page.waitForTimeout(600)
    await node('Лиза').locator('.tn-photo').click({ modifiers: ['Control'] })
    await node('Сергей').locator('.tn-photo').click({ modifiers: ['Control'] })
    const sel = page.getByTestId('tree-selection')
    await expect(sel).toContainText('Выделено: 2')
    await expect(page.getByRole('dialog')).toBeHidden()
    await sel.getByRole('combobox').selectOption({ label: 'Новая группа' })
    await sel.getByRole('textbox', { name: 'Название группы' }).fill('Ветка E2E')
    await sel.getByRole('button', { name: 'Добавить в группу' }).click()
    await expect(sel).toContainText('Готово: 2')
    await sel.getByRole('button', { name: 'Снять выделение' }).click()
    await expect(sel).toBeHidden()
    // «Скрыть умерших» — настройка аккаунта на сервере (F-119)
    const hide = page.getByRole('checkbox', { name: 'Скрыть умерших' })
    await hide.check()
    await expect(node('Лиза')).toBeVisible()
    await page.reload()
    await expect(page.getByRole('checkbox', { name: 'Скрыть умерших' })).toBeChecked()
    await page.getByRole('checkbox', { name: 'Скрыть умерших' }).uncheck()
    // иконка дерева в «Связях» — дерево с человеком в центре (F-129)
    await page.goto(liza)
    await page.getByRole('link', { name: 'Показать в дереве' }).click()
    await expect(page).toHaveURL(/v=tree/)
    await expect(page).toHaveURL(/focus=/)
    await expect(node('Лиза')).toBeInViewport()
    // в центре, узел — около пятой части высоты полотна (±1,5 поколения)
    const canvas = page.locator('.tree-canvas')
    await expect.poll(async () => {
      const c = (await canvas.boundingBox())!
      const n = (await node('Лиза').boundingBox())!
      const dx = Math.abs(n.x + n.width / 2 - (c.x + c.width / 2)) / c.width
      const dy = Math.abs(n.y + n.height / 2 - (c.y + c.height / 2)) / c.height
      const h = n.height / c.height
      return dx < 0.1 && dy < 0.15 && h > 0.12 && h < 0.4
    }).toBe(true)
    await page.goto(liza)
  })

  await test.step('корзина: удалить → восстановить; свадьбу — очистить окончательно', async () => {
    await page.goto(liza)
    await page.getByRole('button', { name: 'В корзину' }).click()
    await expect(page.getByText('Запись в корзине')).toBeVisible()
    await page.getByRole('link', { name: 'Назад' }).click()
    await page.getByRole('link', { name: 'Корзина' }).click()
    const row = page.locator('[data-test="trash-row"]').filter({ hasText: 'Лиза Василькова' })
    await row.getByRole('button', { name: 'Восстановить' }).click()
    await expect(page.getByText('Корзина пуста')).toBeVisible()

    await page.goto(liza)
    await page.locator('.relation').filter({ hasText: 'Свадьба' }).locator('.via-link').first().click()
    await expect(page.locator('h1')).toContainText('Свадьба')
    await page.getByRole('button', { name: 'В корзину' }).click()
    await expect(page.getByText('Запись в корзине')).toBeVisible()
    await page.getByRole('link', { name: 'Корзина' }).click()
    await page.locator('[data-test="trash-row"]').filter({ hasText: 'Свадьба' }).getByRole('button', { name: 'Удалить окончательно' }).click()
    const confirm = page.getByRole('alertdialog')
    await expect(confirm).toContainText('Удалить навсегда?')
    await confirm.getByRole('button', { name: 'Удалить окончательно' }).click()
    await expect(page.getByText('Корзина пуста')).toBeVisible()
  })

  await test.step('журнал: все операции, было → стало, записи очищенного остаются', async () => {
    await page.getByRole('link', { name: 'К списку' }).click()
    await page.getByRole('link', { name: 'Журнал' }).click()
    const audit = page.locator('.audit')
    await expect(audit.locator('[data-test="audit-op"]').first()).toBeVisible()
    for (const s of ['окончательное удаление', 'восстановление', 'в корзину', 'Иванова', 'Василькова', 'Ивановы', 'Воинская часть', 'Брак', 'удалена окончательно']) {
      await expect(audit, s).toContainText(s)
    }
    // фильтр по действию; очищенная запись — с последним названием (F-94)
    await page.getByRole('combobox', { name: 'Действие' }).selectOption({ label: 'окончательное удаление' })
    const purgeOps = audit.locator('[data-test="audit-op"]')
    await expect(purgeOps).toHaveCount(1)
    await expect(purgeOps.first()).toContainText('Свадьба')
    await expect(purgeOps.first()).toContainText('(удалена окончательно)')
    await page.getByRole('combobox', { name: 'Действие' }).selectOption({ label: 'Все действия' })
    await expect(audit.locator('[data-test="audit-op"]').nth(1)).toBeVisible()

    await page.goto(liza)
    await page.getByRole('tab', { name: 'Изменения' }).click()
    const history = page.locator('.audit')
    await expect(history).toContainText('Фамилия')
    await expect(history).toContainText('Дата рождения')
    await expect(history).toContainText('12 марта')
    await expect(history).toContainText('восстановление')
  })

  await test.step('дашборды: новый и переключение', async () => {
    await page.getByRole('link', { name: 'Списки', exact: true }).click()
    const first = page.url()
    await page.getByRole('button', { name: 'Настройки дашборда' }).click()
    const settings = page.getByRole('dialog', { name: 'Настройки дашборда' })
    await settings.getByRole('textbox', { name: 'Название дашборда' }).fill('Основной')
    await settings.getByRole('button', { name: 'Сохранить', exact: true }).click()
    // «Шаблоны»: поля быстрой формы облаками; клик по плашке переносит её между облаками
    await settings.getByRole('tab', { name: 'Шаблоны' }).click()
    const person = settings.getByTestId('quick-kin.core.person')
    const inForm = person.getByRole('list', { name: 'В быстрой форме', exact: true })
    const notInForm = person.getByRole('list', { name: 'Не в быстрой форме', exact: true })
    await expect(inForm.getByRole('button', { name: 'Примечание' })).toBeVisible()
    await inForm.getByRole('button', { name: 'Примечание' }).click()
    await expect(notInForm.getByRole('button', { name: 'Примечание' })).toBeVisible()
    // роль с min ≥ 1 закреплена: у «Родитель — ребёнок» плашка без кнопки
    await expect(settings.getByTestId('quick-kin.core.parent_child').locator('.pill.pinned').first()).toBeVisible()
    // шаг сохраняется сразу (F-132), без кнопки
    await expect(person).toHaveAttribute('data-state', 'saved')
    await expect(settings.getByRole('alert')).toBeHidden()
    await settings.getByRole('tab', { name: 'Доступ' }).click()
    await expect(settings.getByText('Скоро')).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(page.getByRole('combobox', { name: 'Дашборд' }).locator('option').first()).toHaveText('Основной')
    await page.getByRole('button', { name: 'Новый дашборд' }).click()
    const dialog = page.getByRole('dialog', { name: 'Новый дашборд' })
    await dialog.getByLabel('Название дашборда').fill('Работа')
    await dialog.getByRole('button', { name: 'Создать' }).click()
    await expect(page).not.toHaveURL(first)
    await expect(page.getByText('Записей нет')).toBeVisible()
    const switcher = page.getByRole('combobox', { name: 'Дашборд' })
    await expect(switcher.locator('option')).toHaveCount(2)
    await switcher.selectOption({ index: 0 })
    await expect(page).toHaveURL(first)
    await expect(page.locator('main')).toContainText('Лиза')
  })

  await test.step('смена пароля', async () => {
    await page.getByRole('link', { name: LOGIN }).click()
    await page.getByLabel('Текущий пароль').fill(PASSWORD)
    await page.getByLabel('Новый пароль').fill(NEW_PASSWORD)
    await page.getByLabel('Пароль ещё раз').fill(NEW_PASSWORD)
    await page.getByRole('button', { name: 'Сохранить' }).click()
    await expect(page.getByRole('status')).toContainText('Пароль изменён')
  })

  await test.step('выход и вход с новым паролем', async () => {
    await page.getByRole('link', { name: LOGIN }).click()
    await page.getByRole('button', { name: 'Выйти' }).click()
    await expect(page).toHaveURL(/\/login/)
    await page.getByLabel('Логин').fill(LOGIN)
    await page.getByLabel('Пароль', { exact: true }).fill(PASSWORD)
    await page.getByRole('button', { name: 'Войти' }).click()
    await expect(page.getByRole('alert')).toBeVisible()
    await page.getByLabel('Пароль', { exact: true }).fill(NEW_PASSWORD)
    await page.getByRole('button', { name: 'Войти' }).click()
    await expect(page).toHaveURL(/\/d\//)
  })
})
