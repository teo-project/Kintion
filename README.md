<p align="center">
  <img src="assets/logo-mark.svg" alt="Kintion" width="96">
</p>

<h1 align="center">Kintion</h1>

<p align="center"><b>Relationships have history.</b></p>

[Русская версия](README.ru.md)

![Family tree](screenshots/tree-light.png)

## What

Kintion is a self-hosted service for people, the relationships between them, and events over time. Family trees are the first thing it does well, but a "relationship" here is wider than kinship: marriage, friendship, work, study, membership of a community. You run it yourself — one Docker Compose stack with PostgreSQL and a local volume for photos. No cloud, no telemetry.

## Why

It began with a simple wish: to draw my own family. I tried the existing self-hosted tools one by one, and each covered only part of the task. One had a pleasant interface and custom relationship types, but relationships had no dates. Another had start and end dates on relationships, but a poor person card and no way to define your own relationship types. The genealogy tools were built around ancestors: no proper full tree, and relationships limited to the classic parent–child–spouse model. Others looked promising until the real UI showed that relationships were half-finished.

Behind this is a pattern. Tools offer either a good contact book without a time model for relationships, or good genealogy without arbitrary social ties, or a flexible graph without a sensible model of people and their history. What I needed was all of it at once, because real life is not a line between two people: a person changes surname; a couple is dating, then engaged, then married, then divorced; a relationship can join three or more participants, each with their own period of participation; one event can start, end or change a relationship — a wedding is also a change of surname, with witnesses and guests; one date is known only as "12 March" and another only as "sometime in 1985".

No tool I found modelled a relationship as a full historical entity, so Kintion starts from there. **A relationship is a record of its own**, with participants, roles, stages, dates and a history. **A field is a list of values**, not a value, and the top one is simply the current one. **A date may be partial**, and an unknown part stays unknown instead of being guessed. Everything that changes is written to a detailed log. And because "what is a family" differs between cultures and people, the domain knowledge is not in the code at all: it comes from **packs** that you can read, translate and extend.

## Install

Needs Docker with Compose v2.

```sh
cd deploy                              # from the repository root
cp .env.example .env                   # set TEO_KIN_DB_PASSWORD
docker compose up -d --build
docker compose logs app | grep bootstrap_token
```

Using Dockge or Portainer without cloning the repository: see [Dockge / Portainer](docs/deployment.md#dockge--portainer-without-cloning).

Open <http://localhost:8080>, enter the setup token from the log and choose the owner login and password. Ports, configuration, upgrade, backup and restore are in [docs/deployment.md](docs/deployment.md).

## What makes it different

- **Relationships with a life of their own.** A marriage has stages, participants whose role names follow gender (Husband / Wife, Groom / Bride), a start, an end and an event that ended it — not just a line on a chart.
- **Nothing is overwritten.** Every field keeps its history in an order you control; "Vasilkova (Ivanova)" on a tree node is just the first and the current surname.
- **Dates the way people actually know them.** `1985`, `March 1985`, `12 March` — year, month and day are independent, and the database type understands them.
- **A journal of everything**, from the first version: who changed what, from which value to which.
- **Domain in data, not in code.** Entities, templates, fields, roles and caption rules live in language packs; switching the language translates what you have not edited by hand.
- **Yours.** PostgreSQL and files on your disk, one-command backup and restore, schema upgrades by forward migrations.

![Card](screenshots/card-dark.png)

## Read more

| | |
| --- | --- |
| [Features](docs/features.md) | everything that works today |
| [Roadmap](docs/roadmap.md) | what is planned |
| [Data model](docs/data-model.md) | records, field history, partial dates, relationships, groups, change log |
| [Packs](docs/packs.md) | the pack format: entities, templates, fields, roles, caption rules |
| [Architecture](docs/architecture.md) | components, stack, repository layout, API |
| [Deployment](docs/deployment.md) | install, configuration, backup, upgrade |
| [Screenshots](docs/screenshots.md) | gallery |
| [deploy/](deploy/) | Dockerfile, Compose file, `.env` example |
| [pack-storage/](pack-storage/) | the bundled packs |

## License

[Apache License 2.0](LICENSE). You may use, modify and redistribute Kintion, including commercially; keep the copyright and license notices ([NOTICE](NOTICE)) and mark files you changed. The software is provided as is, without warranty, and there is no obligation to maintain or finish it. The name "Kintion" and its logo are not part of the license: a modified or resold copy must not present itself as the official Kintion. The official source is this repository, and the original is free.

Status: early software — a single owner account, local sign-in, no sharing yet.
