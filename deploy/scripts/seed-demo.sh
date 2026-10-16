#!/usr/bin/env bash
# Демо-данные для знакомства с Kintion: заполняет установку через API.
#   deploy/scripts/seed-demo.sh
# Установка должна быть запущена (docker compose … up -d). Если владельца ещё
# нет — он создаётся по токену настройки из лога приложения; иначе — вход.
#
# KIN_COMPOSE        — команда Compose установки (как в lib.sh): из неё берётся
#                      токен настройки;
# KIN_DEMO_URL       — адрес (по умолчанию http://localhost:<порт из TEO_KIN_HTTP_BIND, 8080>);
# KIN_DEMO_LOGIN     — логин владельца (по умолчанию demo);
# KIN_DEMO_PASSWORD  — пароль (по умолчанию demo-password-0.1).
#
# Что появится: Лиза с историей фамилий и днём рождения без года, её мама и
# папа, муж (отношение «Пара» по этапам до брака, свадьба с вложенной
# регистрацией), бывший муж (завершённый брак), дочь, организация с «Работой»
# и коллегами, сообщество с «Членством», два фото с обрезками, группы
# «Семья» / «Работа» / «Гости со стороны невесты» и «Петровы» — и в «Семье»,
# и в «Гостях» (F-105), свои поля, общее поле «Вероисповедание» — «выбор или
# своё» со своим значением (F-96, F-100), дата смерти у папы (F-99), запись
# в корзине и род Лебедевых для дерева (seed-demo-tree.json).
. "$(dirname "$0")/lib.sh"
set -o pipefail

_bind=${TEO_KIN_HTTP_BIND:-8080}
URL=${KIN_DEMO_URL:-http://localhost:${_bind##*:}}
LOGIN=${KIN_DEMO_LOGIN:-demo}
PASSWORD=${KIN_DEMO_PASSWORD:-demo-password-0.1}

# путь Windows в Git Bash: curl получает пути как есть (MSYS_NO_PATHCONV в lib.sh)
TMP=$(cd "$(mktemp -d)" && { pwd -W 2>/dev/null || pwd; })
trap 'rm -rf "$TMP"' EXIT
JAR=$TMP/cookies
CSRF=''

# api МЕТОД ПУТЬ [тело] — JSON-ответ в stdout; код ≥ 400 — ошибка.
api() {
  out=$TMP/out
  if [ $# -ge 3 ]; then
    # тело — файлом: аргументы командной строки на Windows теряют UTF-8
    printf '%s' "$3" >"$TMP/body"
    code=$(curl -sS -o "$out" -w '%{http_code}' -b "$JAR" -c "$JAR" -X "$1" \
      -H 'Content-Type: application/json' -H "X-Kintion-CSRF: $CSRF" --data-binary "@$TMP/body" "$URL$2")
  else
    code=$(curl -sS -o "$out" -w '%{http_code}' -b "$JAR" -c "$JAR" -X "$1" -H "X-Kintion-CSRF: $CSRF" "$URL$2")
  fi
  if [ "$code" -ge 400 ]; then
    log "$1 $2: $code $(cat "$out")"
    exit 1
  fi
  cat "$out"
}

# str КЛЮЧ — первое строковое значение ключа из JSON на входе.
str() {
  sed -n "s/.*\"$1\":\"\([^\"]*\)\".*/\1/p" | head -n 1
}

# refid ССЫЛКА — id записи пакета по временному id (из ответа на входе).
refid() {
  grep -o '"ids":{[^}]*}' | sed -n "s/.*\"$1\":\"\([0-9a-f-]*\)\".*/\1/p"
}

for _ in $(seq 60); do
  curl -sf "$URL/healthz" >/dev/null 2>&1 && break
  sleep 1
done
curl -sf "$URL/healthz" >/dev/null || { log "нет ответа от $URL/healthz"; exit 1; }

if api GET /api/setup | grep -q '"needed":true'; then
  TOKEN=$(compose logs app 2>/dev/null | grep -o '"bootstrap_token":"[^"]*"' | tail -n 1 | cut -d'"' -f4)
  [ -n "$TOKEN" ] || { log "нет токена настройки в логе приложения"; exit 1; }
  ME=$(api POST /api/setup "{\"token\":\"$TOKEN\",\"login\":\"$LOGIN\",\"password\":\"$PASSWORD\"}")
else
  ME=$(api POST /api/login "{\"login\":\"$LOGIN\",\"password\":\"$PASSWORD\"}")
fi
CSRF=$(printf '%s' "$ME" | str csrf)
D=$(api GET /api/dashboards | str id)
log "дашборд $D"

G_FAMILY=$(api POST "/api/dashboards/$D/groups" '{"name":"Семья"}' | str id)
G_WORK=$(api POST "/api/dashboards/$D/groups" '{"name":"Работа"}' | str id)
G_GUESTS=$(api POST "/api/dashboards/$D/groups" '{"name":"Гости со стороны невесты"}' | str id)
# группа в нескольких группах (F-105)
G_PETROV=$(api POST "/api/dashboards/$D/groups" "{\"name\":\"Петровы\",\"parents\":[\"$G_FAMILY\",\"$G_GUESTS\"]}" | str id)

# Два портрета (PNG, нарисованы скриптом): 120×160 и 200×150.
upload() {
  printf '%s' "$1" | base64 -d >"$TMP/photo.png"
  curl -sS -b "$JAR" -H "X-Kintion-CSRF: $CSRF" -F "file=@$TMP/photo.png;type=image/png" "$URL/api/dashboards/$D/files" | str sha256
}
P1=$(upload 'iVBORw0KGgoAAAANSUhEUgAAAHgAAACgCAIAAABIaz/HAAACXklEQVR42u3dwW3DMBBE0akk5aSm1JQzi8g5lbiEVBDDFsXhcPcDvwDx3QQRGo3fBxkSBEADTUADDTQKQANNQAMNNApAA01AAw10Zl+fH9cCeiFuPrpK+gaKqzxxCLeaEG/nVivijdxqSLyFW52VndZqrmyzFsoea6HssRbKHmuh7LEWyh5roeyxFsoea6DPhF537MfP938dYR0N/QTXgx4KvZd4EXdZ6Eni27njoKOI7+UuBb1I+RbrOtBLleeti0AblCetg6Dzlbdb74Q2K89YHwy9Rfmy9anQG5WvWR8JvV35gvV50CHK71oDDXS88lvWQDeDfsU6UPlF68NewYF2QMcqv2INdEvoJ9bhys+tT7pNegR0hWu7QDugj1A2WAMNNNBAx0EfpLzaGmiggQYaaKCBBhpooIEGGmiggQYaaKCBBhpooIEGGmi+GfJxFmiggQaau3dAAw10e+s6/48G2vdH9M7KQFeEzrSuuVoBdFNr58GBLgqdY20+9Z6trG7KY+MoWSvlsXdmr4/y2L5n2ER5JCx0dlAeIVOo5ZVHzrhvbeURtaJcWHkE7oLXIw6FvoU78ES5S/fXuGPPEg39IvoRz38M9OkBDTTQBDTQQEMANNAENNBAowA00AQ00ECjADTQBDTQQBPQQK/s8h79ijGxatC34Cajq7BvlLg6+CaIq5XvRnG1JTZzqzmxjVsoe6wFsYdbEHu4hbLHWih7rIWyx1ooe6wFsYcb6BOgWylPWgtlj7VQ9lgLZY810KnQEF+zFsoea6DzoGGdsQY6DBrQSWugk6ChnLf+A91TbHGz2JfDAAAAAElFTkSuQmCC')
P2=$(upload 'iVBORw0KGgoAAAANSUhEUgAAAMgAAACWCAIAAAAUvlBOAAACgElEQVR42u3cwU3FMBREUZeTiqmEDb2wYUMdVAD6JI7j9+ZYt4LRWVmWx/fnuzS9YQKBJbAElgSWwBJYElgCS2BJYAksgSWBJbAElo6zx3RgTcMEGVgrPBGWDutYe8BCCi+w6pAK5DWQwgusJqoSbA2q2AKrlaretgZVbIHVUFVXW4MqtsBqq6qfrUEVW2A1V9XJFlhggfWEqq+Pt99iC6xpmNYgA6ubqhOkbuIFVhNYF0lN5wVWeVUTSc3lBVZhWDepmmILrKqwblV13RZYJWEtUHXRFlhUsQXWclVXbIFVBtYjqk7bAqsGrAdVnbMFVgFYj6s6YQus3WFtouq/tsACC6w8WFup+pctsMACK+zmfUNVL9py8w4WWGGwtlX1ii2wwAIr6QXp5qr+tuUF6b6VgOW3GbDAAquIqsa2wAILLLDAAgssqtgCCyywwAILLLDAAgsssMACCyywwAILLLDAAgsssMACCyywwAILLLDAAgsssMACCyywwAILLLC8eQcLLLDAAgsssMBiy98NYIEFFlhgsdVeFVhggdXOVu/ZwQILrEa22m8OFlhgdbGVMDhYYIHVwlbI2kGwdrCVM3UWrGdtRe0cB+spW2kjJ8Jabytw4VBYK21lzpsLa42t2G2jYd1tK3nYdFj32QpfFaz5vIwJ1mReBgRrMi+jgTUNmXHAElgCSwJLYAksCSyBJbAksASWwJLAElgCSwJLYAksCSyBJbAksASWwKrbMeOYEaxpmCADa4UnwuJgHU8fsHgiDKxSpEJ4DaTwAquhqq62BlJ4gdWWVD9egyq2wIpQ1cPWoIotsIJUVbc1kMILLLDAoirb1qCKLbByVZWz9QP2NheMzvqZpwAAAABJRU5ErkJggg==')

OP=$(api POST "/api/dashboards/$D/operations" "$(cat <<JSON
{
  "records": [
    { "ref": "liza", "entity": "kin.core.person", "groups": ["$G_FAMILY", "$G_WORK"],
      "start": { "value": { "m": 3, "d": 12 } },
      "note": "Любит горы и старые семейные фотографии",
      "custom_fields": [ { "key": "favorite_color", "type": "text", "name": "Любимый цвет" } ],
      "fields": {
        "name": [ { "value": "Лиза" } ],
        "patronymic": [ { "value": "Сергеевна" } ],
        "surname": [
          { "value": "Василькова", "valid_from": { "y": 2021, "m": 6, "d": 12 },
            "links": [ { "target": "wedding", "anchor": "start" } ] },
          { "value": "Смирнова", "valid_from": { "y": 2008 }, "valid_to": { "y": 2014 } },
          { "value": "Иванова", "note": "девичья" }
        ],
        "gender": [ { "value": "female" } ],
        "photo": [ { "value": "$P1", "meta": { "crop": {
          "card": { "x": 0, "y": 0, "w": 120, "h": 160, "shape": "rect" },
          "tree": { "x": 0, "y": 0, "w": 120, "h": 160, "shape": "rect" },
          "network": { "x": 20, "y": 30, "w": 80, "h": 80, "shape": "circle" } } } } ],
        "phone": [ { "value": "+7 900 111-22-33" } ],
        "email": [ { "value": "liza@example.org" } ],
        "custom.favorite_color": [ { "value": "зелёный" } ]
      } },
    { "ref": "mom", "entity": "kin.core.person", "groups": ["$G_FAMILY"],
      "start": { "value": { "y": 1960, "m": 2, "d": 5 } },
      "fields": { "name": [ { "value": "Марина" } ], "patronymic": [ { "value": "Валентиновна" } ],
        "surname": [ { "value": "Иванова" } ], "gender": [ { "value": "female" } ] } },
    { "ref": "dad", "entity": "kin.core.person", "groups": ["$G_FAMILY"],
      "start": { "value": { "y": 1958 } }, "end": { "value": { "y": 2019, "m": 11 } },
      "fields": { "name": [ { "value": "Сергей" } ], "patronymic": [ { "value": "Петрович" } ],
        "surname": [ { "value": "Иванов" } ], "gender": [ { "value": "male" } ] } },
    { "ref": "ivan", "entity": "kin.core.person", "groups": ["$G_FAMILY"],
      "start": { "value": { "y": 1984, "m": 7, "d": 2 } },
      "custom_fields": [ { "key": "army_unit", "type": "text", "name": "Воинская часть", "history": true } ],
      "fields": { "name": [ { "value": "Иван" } ], "patronymic": [ { "value": "Андреевич" } ],
        "surname": [ { "value": "Васильков" } ], "gender": [ { "value": "male" } ],
        "photo": [ { "value": "$P2" } ],
        "custom.army_unit": [ { "value": "в/ч 1234", "valid_from": { "y": 2002 }, "valid_to": { "y": 2004 } } ] } },
    { "ref": "petr", "entity": "kin.core.person",
      "fields": { "name": [ { "value": "Пётр" } ], "surname": [ { "value": "Смирнов" } ], "gender": [ { "value": "male" } ] } },
    { "ref": "anya", "entity": "kin.core.person", "groups": ["$G_FAMILY"],
      "start": { "value": { "y": 2016, "m": 9, "d": 1 } },
      "fields": { "name": [ { "value": "Аня" } ], "surname": [ { "value": "Василькова" } ], "gender": [ { "value": "female" } ] } },
    { "ref": "kolya", "entity": "kin.core.person", "groups": ["$G_PETROV"],
      "fields": { "name": [ { "value": "Николай" } ], "surname": [ { "value": "Петров" } ], "gender": [ { "value": "male" } ] } },
    { "ref": "tanya", "entity": "kin.core.person", "groups": ["$G_PETROV"],
      "fields": { "name": [ { "value": "Татьяна" } ], "surname": [ { "value": "Петрова" } ], "gender": [ { "value": "female" } ] } },
    { "ref": "olga", "entity": "kin.core.person", "groups": ["$G_WORK"],
      "fields": { "name": [ { "value": "Ольга" } ], "surname": [ { "value": "Кузнецова" } ], "gender": [ { "value": "female" } ] } },
    { "ref": "dmitry", "entity": "kin.core.person", "groups": ["$G_WORK"],
      "fields": { "name": [ { "value": "Дмитрий" } ], "surname": [ { "value": "Орлов" } ], "gender": [ { "value": "male" } ] } },
    { "ref": "org", "template": "kin.core.organization", "groups": ["$G_WORK"],
      "start": { "value": { "y": 1998 } },
      "fields": { "name": [ { "value": "Ромашка" } ], "phone": [ { "value": "+7 495 000-00-00" } ],
        "website": [ { "value": "romashka.example" } ], "actual_address": [ { "value": "Москва, ул. Полевая, 7" } ] } },
    { "ref": "club", "template": "kin.core.community",
      "fields": { "name": [ { "value": "Клуб «Вершина»" } ], "topic": [ { "value": "Походы выходного дня" } ] } },

    { "ref": "pc_mom", "template": "kin.core.parent_child" },
    { "ref": "pc_dad", "template": "kin.core.parent_child" },
    { "ref": "pc_liza_anya", "template": "kin.core.parent_child" },
    { "ref": "pc_ivan_anya", "template": "kin.core.parent_child" },
    { "ref": "pair", "template": "kin.core.relationship", "start": { "value": { "y": 2018, "m": 3 } },
      "fields": { "stage": [
        { "value": "marriage", "valid_from": { "y": 2021, "m": 6, "d": 12 } },
        { "value": "cohabitation", "valid_from": { "y": 2019, "m": 5 } },
        { "value": "dating", "valid_from": { "y": 2018, "m": 3 } } ] } },
    { "ref": "reg", "template": "kin.core.registration", "start": { "value": { "y": 2021, "m": 6, "d": 12 } },
      "fields": { "place": [ { "value": "Москва, Грибоедовский ЗАГС" } ] } },
    { "ref": "wedding", "template": "kin.core.wedding", "start": { "value": { "y": 2021, "m": 6, "d": 12 } },
      "custom_fields": [ { "key": "registration", "type": "ref", "name": "Регистрация" } ],
      "fields": { "place": [ { "value": "Подмосковье, усадьба «Липки»" } ],
        "custom.registration": [ { "value": "reg" } ] } },
    { "ref": "ex", "template": "kin.core.relationship",
      "start": { "value": { "y": 2008 } }, "end": { "value": { "y": 2014, "m": 5 } },
      "fields": { "stage": [ { "value": "marriage", "valid_from": { "y": 2008 } } ] } },
    { "ref": "job", "template": "kin.core.work", "start": { "value": { "y": 2015, "m": 2 } } },
    { "ref": "membership", "template": "kin.core.membership", "start": { "value": { "y": 2019 } } }
  ],
  "participants": [
    { "container": "pc_mom", "entity": "mom", "role": "parent" },
    { "container": "pc_mom", "entity": "liza", "role": "child" },
    { "container": "pc_dad", "entity": "dad", "role": "parent" },
    { "container": "pc_dad", "entity": "liza", "role": "child" },
    { "container": "pc_liza_anya", "entity": "liza", "role": "parent" },
    { "container": "pc_liza_anya", "entity": "anya", "role": "child" },
    { "container": "pc_ivan_anya", "entity": "ivan", "role": "parent" },
    { "container": "pc_ivan_anya", "entity": "anya", "role": "child" },
    { "container": "pair", "entity": "liza", "role": "partner" },
    { "container": "pair", "entity": "ivan", "role": "partner" },
    { "container": "reg", "entity": "liza", "role": "partner" },
    { "container": "reg", "entity": "ivan", "role": "partner" },
    { "container": "wedding", "entity": "liza", "role": "partner" },
    { "container": "wedding", "entity": "ivan", "role": "partner" },
    { "container": "wedding", "entity": "olga", "role": "witness" },
    { "container": "wedding", "entity": "dmitry", "role": "guest" },
    { "container": "wedding", "entity": "kolya", "role": "guest" },
    { "container": "wedding", "entity": "tanya", "role": "guest" },
    { "container": "ex", "entity": "liza", "role": "partner" },
    { "container": "ex", "entity": "petr", "role": "partner" },
    { "container": "job", "entity": "org", "role": "employer" },
    { "container": "job", "entity": "liza", "role": "colleague" },
    { "container": "job", "entity": "olga", "role": "colleague" },
    { "container": "job", "entity": "dmitry", "role": "manager" },
    { "container": "membership", "entity": "club", "role": "organization" },
    { "container": "membership", "entity": "liza", "role": "member" },
    { "container": "membership", "entity": "ivan", "role": "member" },
    { "container": "membership", "entity": "olga", "role": "organizer" }
  ]
}
JSON
)")
if printf '%s' "$OP" | grep -q '"errors":\[{'; then
  log "не сохранились значения: $(printf '%s' "$OP" | grep -o '"errors":\[.*\]')"
  exit 1
fi
LIZA=$(printf '%s' "$OP" | refid liza)
IVAN=$(printf '%s' "$OP" | refid ivan)

# Общее поле людей дашборда (F-96) — «выбор или своё» (F-100): у Лизы —
# вариант, у Ивана — своё значение.
# первое вхождение — сама запись (str берёт последнее, а за ней — связанные)
PERSON_TPL=$(api GET "/api/records/$LIZA" | grep -o '"template_id":"[^"]*"' | head -n 1 | cut -d'"' -f4)
api PUT "/api/dashboards/$D/templates/$PERSON_TPL/custom_fields" '{"if_rev":0,"custom_fields":[
  {"key":"faith","type":"choice","name":"Вероисповедание","free":true,
   "options":[{"key":"orthodox","name":"Православие"},{"key":"none","name":"Нет"}]}]}' >/dev/null
api POST "/api/records/$LIZA/fields/shared.faith/values" '{"if_rev":0,"value":"orthodox"}' >/dev/null
api POST "/api/records/$IVAN/fields/shared.faith/values" '{"if_rev":0,"value":"Буддизм","meta":{"free":true}}' >/dev/null

# В корзине — запись, созданная по ошибке (восстанавливается из корзины).
TRASH=$(api POST "/api/dashboards/$D/operations" '{"records":[{"ref":"t","entity":"kin.core.person","fields":{"name":[{"value":"Дубль Лизы"}],"surname":[{"value":"Василькова"}]}}],"participants":[]}' | refid t)
api DELETE "/api/records/$TRASH" >/dev/null

# Дерево (F-110…F-119): род Лебедевых — 43 человека в семи поколениях:
# братья без родителей, поздний ребёнок, петля (Ольга замужем за братом
# деда), два брака, отчим, приёмная дочь, опекун, родители без отношения,
# предки из других родов, умершие. Данные — seed-demo-tree.json.
G_LEBEDEV=$(api POST "/api/dashboards/$D/groups" '{"name":"Лебедевы"}' | str id)
api POST "/api/dashboards/$D/operations" "$(sed "s/\$G_LEBEDEV/$G_LEBEDEV/g" "$(dirname "$0")/seed-demo-tree.json")" >/dev/null

log "готово: $URL/d/$D/r/$LIZA"
log "вход: $LOGIN / $PASSWORD"
