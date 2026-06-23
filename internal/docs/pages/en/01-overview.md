# Overview

Go-Liga is a small management app for school Go clubs. It keeps track of the players in a group along with their current strength (GoR) and records the results of the games played in the weekly club session. The children enter their games live on a tablet; the coach handles everything around it via the MCP interface from within an AI chat.

## Who is involved?

- **Players** are pure name entries with a current GoR. They have no login and no session — they are just records to which games are assigned.
- **Admins** are people with an account at [id.levinkeller.de](https://id.levinkeller.de). They can administer one or more groups, invite further admins, and handle all administrative tasks via the MCP tool.
- **MCP clients** are AI chats like [Claude.ai](https://claude.ai). They speak the MCP protocol, authenticate against our app on first connection, and can then act on behalf of the signed-in admins.

## Three interfaces

- **Tablet UI** at `/g/{slug}/play` — colorful tiles, a multi-step wizard, handicap calculator, and game entry. Meant for the children; iOS Guided Access locks the app.
- **Admin overview** at `/` and `/g/{slug}` — ranking, player list, admin management. A read-only display; all changes go through MCP.
- **MCP endpoint** at `/mcp` — JSON-RPC 2.0 with OAuth authentication. This is where Claude.ai connects.

## What's next?

- **[Strength](/docs/ranking)** — how the GoR value works and how we keep it updated (with formulas and sources).
- **[Handicap & Komi](/docs/handicap)** — how we turn the strength difference into a stones/komi recommendation.
- **[MCP server](/docs/mcp)** — how the admin chat is connected and which tools exist.
- **[Tablet workflow](/docs/tablet)** — how the wizard is played through.
- **[Authentication](/docs/auth)** — how OIDC and OAuth work together.
- **[Deployment](/docs/deploy)** — how the code gets into the cluster.
