#!/usr/bin/env node
// Тексты лицензий сторонних библиотек, попадающих в сборку (F-91):
// web/licenses/THIRD_PARTY_LICENSES.txt — Go встраивает его в бинарник и отдаёт
// только вошедшим (GET /api/licenses), интерфейс даёт ссылку со страницы «Аккаунт».
// Версии библиотек в файл не пишутся: точный список версий подсказывает, какие
// известные уязвимости пробовать.
//   node web/scripts/third-party-licenses.mjs
// Фронтенд — пакеты, чьи модули реально вошли в бандл (сборка Vite без записи).
// Go — модули из `go list -deps` бинарника kintion и стандартная библиотека;
// без Go на хосте — через контейнер tools (deploy/compose.dev.yml).
// Запускать после обновления зависимостей (package-lock.json, go.mod).
import { spawnSync } from 'node:child_process'
import { mkdirSync, readdirSync, readFileSync, writeFileSync } from 'node:fs'
import { dirname, join, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const WEB = fileURLToPath(new URL('..', import.meta.url))
const ROOT = resolve(WEB, '..')
const OUT = join(WEB, 'licenses', 'THIRD_PARTY_LICENSES.txt')
const LICENSE_FILE = /^(licen[cs]e|copying|notice)([.-].*)?$/i
const RULE = '='.repeat(78)

/** Пакеты npm, модули которых попали в бандл: имя → каталог пакета. */
async function bundledPackages() {
  const { build } = await import('vite')
  const ids = new Set()
  await build({
    root: WEB,
    configFile: join(WEB, 'vite.config.ts'),
    logLevel: 'warn',
    build: { write: false, emptyOutDir: false },
    plugins: [{
      name: 'kintion-collect-modules',
      generateBundle(_, bundle) {
        for (const chunk of Object.values(bundle)) {
          for (const id of chunk.moduleIds ?? []) ids.add(id)
        }
      },
    }],
  })
  const pkgs = new Map()
  for (const raw of ids) {
    const id = raw.replace(/^\0/, '').split('?')[0].replaceAll('\\', '/')
    const m = id.match(/^(.*\/node_modules\/)((?:@[^/]+\/)?[^/]+)\//)
    if (!m) continue
    pkgs.set(m[2], m[1] + m[2])
  }
  return [...pkgs.entries()].sort(([a], [b]) => a.localeCompare(b))
}

function licenseTexts(dir) {
  return readdirSync(dir)
    .filter(f => LICENSE_FILE.test(f))
    .sort()
    .map(f => readFileSync(join(dir, f), 'utf8').trim())
}

async function frontend() {
  const parts = []
  for (const [name, dir] of await bundledPackages()) {
    const pkg = JSON.parse(readFileSync(join(dir, 'package.json'), 'utf8'))
    const texts = licenseTexts(dir)
    if (texts.length === 0) {
      throw new Error(`${name}: нет файла лицензии — добавьте текст вручную в скрипт`)
    }
    parts.push(section(`${name} (npm) — ${pkg.license ?? 'см. текст'}`, texts))
  }
  return parts
}

// В контейнере или на хосте с Go: модули бинарника и тексты их лицензий,
// разделённые строками «@@@ путь версия».
const GO_SCRIPT = `set -e
go mod download
echo "@@@ std $(go env GOVERSION)"
cat "$(go env GOROOT)/LICENSE"
go list -deps -f '{{with .Module}}{{if not .Main}}{{.Path}} {{.Version}} {{.Dir}}{{end}}{{end}}' ./server/cmd/kintion \\
  | sort -u | while read -r path version dir; do
    echo "@@@ $path $version"
    found=0
    for f in "$dir"/LICENSE* "$dir"/LICENCE* "$dir"/COPYING* "$dir"/NOTICE*; do
      [ -f "$f" ] && { cat "$f"; echo; found=1; }
    done
    [ "$found" = 1 ] || echo "!!! нет файла лицензии"
  done`

function goLicensesRaw() {
  const hasGo = spawnSync('go', ['version'], { stdio: 'ignore' }).status === 0
  const r = hasGo
    ? spawnSync('sh', ['-c', GO_SCRIPT], { cwd: ROOT, encoding: 'utf8', maxBuffer: 64 << 20 })
    : spawnSync('docker', ['compose', '-f', 'deploy/compose.dev.yml', 'run', '--rm', '--no-deps', '-T', 'tools', 'sh', '-c', GO_SCRIPT],
      { cwd: ROOT, encoding: 'utf8', maxBuffer: 64 << 20 })
  if (r.status !== 0) throw new Error(`Go-лицензии не собраны:\n${r.stderr}`)
  return r.stdout
}

function goModules() {
  const parts = []
  for (const block of goLicensesRaw().split(/^@@@ /m).slice(1)) {
    const nl = block.indexOf('\n')
    const [path] = block.slice(0, nl).trim().split(' ')
    const text = block.slice(nl + 1).trim()
    if (text.includes('!!! нет файла лицензии')) throw new Error(`${path}: нет файла лицензии`)
    const title = path === 'std' ? 'Go standard library' : `${path} (Go)`
    parts.push(section(title, [text]))
  }
  return parts
}

function section(title, texts) {
  return `${RULE}\n${title}\n${RULE}\n\n${texts.join('\n\n---\n\n')}\n`
}

const head = `Kintion — лицензии сторонних библиотек / Third-party licenses

Сборка Kintion включает перечисленные ниже библиотеки и шрифт. Каждая
распространяется на условиях своей лицензии, текст которой приведён полностью.
`
const body = [...(await frontend()), ...goModules()]
mkdirSync(dirname(OUT), { recursive: true })
writeFileSync(OUT, `${head}\n${body.join('\n')}`.replaceAll('\r\n', '\n'))
console.log(`${body.length} библиотек → ${OUT}`)
