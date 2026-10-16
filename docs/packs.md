# Packs

A **pack** is a set of JSON files that describes a domain: which entities exist, which templates they have, which fields and roles those templates define, how captions are built, how names change with gender. The application contains no hard-coded knowledge of "person" or "marriage" — it reads packs. The bundled packs are in [`pack-storage/`](../pack-storage).

Format marker in every file: `"format": "kintion-pack/0"`. The format is a v0 draft and may still change.

## Bundled packs

| Pack | Content |
| --- | --- |
| `kin.core` | entities Person, Organization, Relationship, Event; templates: Person; Organization, Community; Couple (the default Relationship), Parent–child, Sibling, Friend, Acquaintance, Commune, Work, Study, Membership; Wedding, Registration, Divorce, Birth, Death |
| `kin.christian.orthodox` | requires `kin.core`; adds *Church marriage* (a relationship), *Wedding sacrament* and *Church dissolution* (events) |

Each pack exists per **language** (`en`, `ru`). The instance language is set with `TEO_KIN_LANGUAGE`. Words (names of fields, roles, options) are written straight into the language version of the files; the keys of limited values (`male`, `marriage`) are English in every language. Switching the language translates everything the user has not edited by hand. Enabled packs are listed in `TEO_KIN_PACKS`; dependencies are added automatically.

Fields and options that only make sense in some languages are marked `localized_field` / `localized_options` (for example a tax id in `ru`, VAT ID in `en`; legal forms of companies per country). Values of a field that does not exist in the current language are kept dormant and come back when the language is switched back.

Packs are installed by the administrator of the instance only. Data of a template whose pack has been removed stays in the database but stops working. Custom templates outlive their pack.

## Layout

```text
<language>/<pack>/
  <pack>.pack.json                      pack manifest
  entities/<entity>.json                entities (if the pack introduces them)
  templates/<entity>/<template>.json    templates, in the folder of the entity's full id
```

A template of a foreign entity lives in that entity's folder: `kin.christian.orthodox` puts *Church marriage* into `templates/kin.core.relationship/`.

## Manifest

| Key | Meaning |
| --- | --- |
| `format`, `id`, `version`, `language` | format version, pack id (`kin.core`), pack version, language of this copy |
| `name`, `description` | for humans |
| `requires` | packs this one cannot work without |
| `files` | all files of the pack, relative to its folder |

## Entity

| Key | Meaning |
| --- | --- |
| `id` | full id, `kin.core.person` |
| `participant`, `container` | may take part in a container / may contain participants |
| `period` | default labels of the system dates, e.g. `{ "start": "Date of birth", "end": "Date of death" }`; `"end": null` makes the record instantaneous (an event); `{ "name": …, "show": "filled" }` hides an empty date |
| `stages` | `allowed` — templates may define stages |
| `links` | whether the "Relations" panel shows other participants of this container |
| `system` | names of system fields (stage, caption, note, sensitive, groups) |

## Template

A template whose id equals its entity id is the **default template**: a record is created from it when none is chosen.

| Key | Meaning |
| --- | --- |
| `id`, `entity`, `name` | template id, full entity id, name |
| `title` | rule that builds the caption of a record (see below) |
| `tree_title` | caption of a tree node, same grammar; falls back to `title` |
| `quick_fields` | fields of the quick-add form; the dashboard owner can override them per dashboard |
| `sort_by` | field keys that order lists (current value, case-insensitive, empty last) |
| `end_flag` | key of a yes/no field that marks the record as finished ("Deceased") |
| `fields` | fields |
| `roles` | roles of participants (containers) |
| `stages` | stages of a relationship |
| `end_events` | event templates that may end a relationship |
| `links`, `links_rank` | participation in the "Relations" panel and its weight |
| `tree_link` | whether and how the template appears in the tree; `line_by` picks the line style from a choice field |
| `effects` | suggestions without code (see below) |
| `ui.quick_add` | quick-add buttons: "Add father", "Add mother", "Add child"… |

### Field

```json
{ "key": "surname", "type": "text", "name": "Surname", "history": true, "section": "main", "surname": true }
```

| Key | Meaning |
| --- | --- |
| `type` | `text`, `number`, `partial_date`, `bool`, `choice`, `ref`, `image`, `place`, `phone`, `email`, `url`, `contact` |
| `history` | the value changes over time; the top one is current |
| `multiple` | several values valid at once (phones) |
| `section` | `main` or `additional` on the card |
| `show` | `always` (default) or `filled` — an empty field is hidden and added with "Add" |
| `sensitive` | marks a sensitive field |
| `surname` | this is the surname field of the record, used by groups that add records by surname |
| `options` | for `choice`: `[{ key, name }]`, with optional `tree_line` and role `forms` |
| `free` | for `choice`: a custom value is allowed |
| `localized_options`, `localized_field` | language-dependent options / fields |
| `meta` | allowed extension data of a value (highlight color, photo crop…) |

### Roles and forms

```json
{ "key": "parent", "entities": ["kin.core.person"], "min": 1, "max": 1,
  "forms": { "by": "gender", "male": "Father", "female": "Mother", "default": "Parent" } }
```

`forms.by` names the property of the participant that selects the form of the role name; `default` is mandatory. A choice option or a stage may carry its own `forms` (adopted → *Adoptive father / Adoptive mother*).

### Caption rules

A caption is built at display time from an array of elements concatenated in order:

| Element | Gives |
| --- | --- |
| `{ "freeedit": true }` | the caption may be typed by hand and then replaces the rule |
| `{ "field": "surname" }` | the stored value of a field |
| `{ "field": "@stage" }` | the resolved value: `@` turns a stored key into text by the template rules |
| `{ "template-property": "name" }` | a property of the template, e.g. its name |
| `{ "string": " · " }` | literal text |
| `{ "pattern": " ({maiden})", "data": { … } }` | a short pattern; `[...]` parts drop out when a value is empty |
| `"if"` / `"not"` | on any element: insert only if a value exists / does not exist |

Addresses: `surname` current value, `surname[n]` n-th in the manual order (`[-1]` the first ever), `@x` resolved text, `start.year` part of a value, `surname.note` note of a value, `custom.key` a custom field, `shared.key` a shared field. Conditions are a string ("has a value") or an object with `eq`, `ne`, `gt`, `lt`, `count`, `all`, `any`.

For example, a person's tree caption appends the first surname in brackets only when it differs from the current one:

```json
{ "pattern": " ({maiden})",
  "data": { "maiden": { "field": "surname[-1]" } },
  "if": { "ne": ["surname[0]", "surname[-1]"] } }
```

### Effects (suggestions without code)

Effects never change data by themselves; they offer an action and the user confirms.

| `type` | Offers |
| --- | --- |
| `suggest_relationship` | create a relationship from the participants of an event, or move an existing one to a stage |
| `suggest_end` | end a relationship or a stage |
| `suggest_field_value` | add a field value to a participant |
| `suggest_period` | write the event date into a participant's start or end and link it to the event |
| `decorate` | style a participant (for example a mourning mark) from the date of an event |

## Extensions (planned)

The core pack contains no active code. Sandboxed JavaScript logic for templates and UI slots, installed by the administrator, are on the [roadmap](roadmap.md).
