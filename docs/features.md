# Features

## What works today (v0.2)

**Records and relationships**
- Four entities — **Person, Organization, Relationship, Event** — with ready-made templates: couple, parent–child, sibling, friend, acquaintance, commune, work, study, membership; wedding, registration, divorce, birth, death; community.
- A relationship is a container with participants and **roles whose names depend on the participant** (Father / Mother, Husband / Wife, Groom / Bride…), a start and an end, stages, and a "past" mark for finished ones.
- **Quick add** from a card or a tree node: add a father, mother, child, partner — or pick an existing record with suggestions.

**Fields with history**
- Every field is an **ordered list of values**; the top one is current. Surnames before and after marriage, addresses, jobs — all kept, with notes.
- **Partial dates** as a real type: `1985`, `March 1985`, `12 March` without a year.
- **Eras**: named eras, dashboard-defined order, optional year bounds and reverse counting; a current year for a fictional world.
- Field types: text, choice (also "choice or your own value"), yes/no, place, phone, e-mail, URL, image, file attachment and formatted text (Markdown with preview). Custom fields per record and shared fields per template. Place custom fields in existing card sections or create a section for one record or a template. Images open in a full-size viewer. A "Deceased" flag for when the date of death is unknown.

**Family tree**
- Generation tree on a canvas: couples, children, transfer lines for repeated people, minimap, "fit all".
- Full names, photos, relationship labels on lines, line highlighting on hover, a mourning ribbon, "hide the deceased", filter by group, generation depth.

**Organizing and safety**
- **Dashboards** — closed data containers (family, friends, a project).
- **Groups** — labels and filters, nesting, created on the fly, auto-add by surname.
- **Selection mode** in the tree and in lists; lists sorted by template rules.
- **Sharing**: the owner grants view or edit access to a group to another account by nickname; the guest confirms the first invitation and sees only the records of that group and of the groups inside it — everything else behaves as if it did not exist. A guest with edit access edits those records; whether a guest may create new ones is a separate switch in the access settings, and new records receive a dedicated group managed by the owner. See [Sharing](sharing.md) for permissions and limits.
- **Accounts**: registration by an invitation link from the admin or from any user (or open, or closed — `TEO_KIN_REGISTRATION`); a nickname to be invited by.
- A **detailed change log** from the first version.
- **Trash** with restore. Backup and restore with one Compose command.
- Light and dark theme; English and Russian interface and packs.

See [screenshots.md](screenshots.md).

What is planned — [roadmap.md](roadmap.md).
