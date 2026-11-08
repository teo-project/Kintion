# Sharing a dashboard

Registration on the server and access to a dashboard are separate steps. Creating an account does not grant access to anyone else’s data. Each new account starts with its own dashboard.

## Invite an account to the server

With the default `TEO_KIN_REGISTRATION=invite`, the administrator creates a registration link under **Account → Accounts**. Each link is valid for seven days and can be used once. The recipient chooses a login, nickname and password.

Other modes: `invite-all` lets any user create registration links; `open` allows registration from the sign-in page; `closed` lets the administrator create accounts and hand over credentials. All registration stays closed until initial setup is complete.

The nickname can be set during registration or under Account. It is an invitation address, not a searchable directory: ask the person for their nickname.

## Share selected groups

1. As the dashboard owner, create groups and put the records you want to share in them.
2. Open **Dashboard settings → Access**, enter the other account’s nickname, choose a group and view or edit permission.
3. The recipient accepts the first invitation above their dashboard list. Before acceptance there is no access. Further grants in that dashboard apply immediately.
4. Manage the guest’s groups and permissions from their access card. A guest without a nickname is shown by login; you can still manage their permissions.

Access to a group includes its nested groups, along every nesting path. A visible relationship does not make an otherwise hidden participant visible. The tree, search, counts, cards and files follow the same permissions. A guest can view dashboard settings but cannot change them or open the Access tab.

## Let a guest edit or create

Edit permission on a group lets the guest edit its records. Labels for groups outside their edit permission are preserved. A guest can choose only visible participants and files available to them.

The separate creation control uses a dedicated **Created by …** group. It is created when the invitation is accepted; new records created by that guest receive its label. Direct edit permission on this group permits creation; direct view permission disables creation and leaves that group’s records available for viewing (other edit grants still apply). An initial invitation with edit access enables creation; the owner can change it later.

The dedicated group is hidden from the guest’s group picker, but the owner sees it and can manage its records. It cannot be deleted while the guest is a dashboard member. Access remains subject to the owner’s later changes.

## Revoke access and understand the limits

The owner can revoke individual grants or remove the guest entirely. Removing one grant does not remove access still granted through another group or a nesting path.

- Only the owner deletes, restores or purges records and manages groups, templates, eras and permissions.
- Guests see only their own actions in the change log. The owner sees all dashboard actions.
- A single-value reference to a hidden record is shown as **Set** and cannot be changed by the guest. Field revisions are shared, so a revision can increase after a change the guest cannot see.
- A sensitive flag currently does **not** hide records or fields. The mode without sensitive data is planned.
- Later grants have no notification yet. Leaving a dashboard is available through the API; in the interface, ask the owner to remove access.

[Features](features.md) · [Release notes](../RELEASE_NOTES.md)
