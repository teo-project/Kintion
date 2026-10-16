# Data model

## Vocabulary

| Term | Meaning | Example |
| --- | --- | --- |
| **entity** | a type of record | Person, Organization, Relationship, Event |
| **template** | a configuration on top of an entity | Couple, Wedding, Community |
| **record** | a concrete instance | Liza Vasilkova; the wedding of Anna and Ivan |
| **view** | a visual presentation of data | Tree, (planned: Graph, Timeline, Map) |
| **scene** | a saved, configured view | "The Vasilkovs" on the Tree |

Everything is a record. Any record may **participate** in another and some may **contain** others: a *Relationship* or an *Event* is a container whose participants have roles. A person is a participant; an organization can be both.

## Accounts, dashboards, groups

- An account owns **dashboards**. A dashboard is a closed container of data: records of one dashboard never mix with another.
- A **group** is a label inside a dashboard (family, work). Groups can be nested many-to-many and are used as filters in lists and in the tree. A group may add records **automatically by surname**: it holds a list of surnames, and every record that has any of them anywhere in its surname history (case-insensitive) joins the group; nothing is removed automatically.
- Every record has a `sensitive` flag. It is stored from the first version; rules that use it arrive with sharing.

## Records and tables

PostgreSQL holds a relational skeleton. Everything that is a link is a table with foreign keys; the extensible content of values is JSONB.

| Table | Content |
| --- | --- |
| `account`, `session`, `bootstrap` | owner, sessions, one-time setup token |
| `dashboard`, `dashboard_template` | dashboards and the templates available in each |
| `pack` | installed packs and their versions |
| `entity` | records: entity and template, `start`/`end` (partial dates), note, manual caption, custom fields, soft-delete marks |
| `grp`, `grp_link`, `entity_group` | groups, nesting, membership |
| `participation` | a participant in a container, with a role, validity period and order |
| `field_entry` | values of fields: typed columns (`v_text`, `v_num`, `v_date`, `v_bool`, `v_ref`), `meta`, period, note, manual order |
| `field_rev` | per-field revision counter for conflict detection |
| `entry_link` | links of a value to other records |
| `file` | content-addressed files |
| `audit_log` | the change log |
| `schema_info` | schema and application version |

## Fields are lists of values

Every field is an **ordered list of values**. The order is manual; **the top value is the current one**. A value may carry a start and an end (partial dates), a note, and extension data defined by the template (`meta`: a highlight color, a photo crop…).

Example — the surnames of one person:

| # | Value | Note |
| --- | --- | --- |
| 1 (current) | Vasilkova | |
| 2 | Ivanova | maiden name |

A template can refer to values by position: `surname[0]` is the current one, `surname[-1]` is the first ever recorded. That is how a tree node shows "Liza Vasilkova (Ivanova)".

Duplicates are allowed. "Make current" is a UI action that moves a value to the top; there is no separate flag. Contradictions go into the note ("per passport: 1923"), not into a rule.

A field may also be **multiple** (several values valid at once, e.g. phones) — then there is no "current" value and the order is only for display — or have history switched off.

Validation errors in one field never fail the saving of the whole record.

### Field types

`text`, `number`, `partial_date`, `bool`, `choice`, `ref`, `image`, `place`, `phone`, `email`, `url`, `contact`.

- `choice` may be "free": a value from the options **or your own**. An option is stored by key, a custom value by text with a mark in `meta`.
- `place` is a data type, not an entity.
- `image` values carry a separate crop per place of display (tree portrait, square, card).
- A record can also have **custom fields** (its own) and a template can have **shared custom fields** added to all records of that template in a dashboard.

## Partial dates

A date is a composite type `(year, month, day)` where **each part may be missing**, as long as at least one is present:

| Entered | Stored |
| --- | --- |
| `1985` | year only |
| `March 1985` | year and month |
| `12 March` | day and month, year unknown |

The year, the month and the day are entered separately. The database type is a domain over a composite type with a validity check (it knows that `29 February` is valid when the year is unknown or a leap year). The same rules are implemented in Go and in TypeScript and are tested against one shared set of cases. Ordering and "possibly earlier" operators are intentionally not defined yet.

## Start and end

Every record has system `start` and `end` dates; the template gives them names: *Date of birth / Date of death* for a person, *Start / End* for a relationship. They can be bound to an event (for example the birth event of a child sets the start of the parent–child relationship). An empty end can be hidden until needed. A template can declare an `end_flag` — a yes/no field such as "Deceased" — so that a record counts as finished even when no date is known.

## Relationships

A relationship is a record of its own, with participants:

- **Participation** = container + participant + role, optionally with its own period.
- Roles are defined by the template. Their **names depend on the participant**: the parent role is shown as *Father* or *Mother* by the participant's gender, with a neutral default.
- A relationship may have **stages** (e.g. *Dating → Engaged → Marriage*) kept as a field with history, so the development of a relationship is the history of one record. Stages can bring their own role names (*Husband / Wife* in marriage).
- A finished relationship is shown as **past**.
- A relationship can end by an **event** (divorce, death) — templates list which events are allowed.
- Parent–child links are typed (biological, adopted, step, guardian) and shown with different lines in the tree.

The **Relations** panel of a card lists, from the point of view of the opened record, other participants and the records through which they are related ("Wife: Natalia (via Marriage)"), ordered by a template-defined weight and then by start date.

## Change log

Two different histories exist:

| | History of values | Change log |
| --- | --- | --- |
| What | what happened in life | who changed what and when |
| Example | surnames: Vasilkova, Ivanova | "demo added the surname Ivanova" |
| Mutability | freely editable: order, dates, duplicates | append-only |
| Order | manual | by time |

Each change-log row stores the operation (several rows of one operation are grouped), the actor, the source, the request id, the application and schema versions, the record, the object, and the **before and after** values. The log is detailed from the first version.

## Deletion

Deleting a record moves it to the **trash** (soft delete); it can be restored or purged for good. References to a purged record are cleaned up; a purge is itself logged.
