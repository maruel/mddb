# mddb - Markdown Document & Table System

A Notion-like document and table system for managing your information. Store everything locally as files, no external services required.

[![codecov](https://codecov.io/gh/maruel/mddb/graph/badge.svg?token=Q4SYDP0B00)](https://codecov.io/gh/maruel/mddb)

## Why?

an anecdote:

<a href="https://x.com/anothercohen/status/2014136100894199968"><img src="https://github.com/user-attachments/assets/3e3bbf18-9ffa-4669-9f43-b7e9828e41ef" style="width: 400px;" /></a>

and then:

<a href="https://x.com/ivanhzhao/status/2003510041010405790"><img src="https://github.com/user-attachments/assets/263f10ac-8f5f-484f-a1f8-415d49896de8" style="width: 400px;" /></a>

\-\- Ivan Zhao, Notion CEO


## What is mddb?

mddb lets you:
- 📝 **Create and edit documents**: Write in markdown with live preview (Google Docs)
- 📊 **Build tables**: Store structured data with typed columns and cross table references (Google Sheets)
- 🗂️ **Organize information**: Create folders and nested structures (Google Drive)
- ⚡ **Auto-save**: Changes save automatically as you work
- 📁 **Keep it local**: All data stored as files on your computer
- 🔄 **Git-native**: Every change is a git commit—sync to any remote, track history, branch and merge
- 🔍 **Search**: Search across documents, tables, and folders
- 🚀 **Designed for LLM agents**: storage is easy for an agent to navigate

> **Note**: mddb uses a two-level hierarchy:
> - **Organization**: Billing and administrative entity (like a company or team)
> - **Workspace**: Isolated content container within an organization (like a project or department)
> - **Table** instead of Notion's "Database" (a collection of records with columns)
>
> Each workspace has its own pages, tables, and git remote for syncing.

## Getting Started

See [SELF_HOSTING.md](SELF_HOSTING.md) to run it yourself for free and access it securely from Tailscale.

## File Structure

All data is stored as plain files. Each workspace is a separate git repository:

```
data/
├── db/                             # Identity database (users, orgs, memberships)
└── <workspace-id>/                 # Each workspace is independent
    ├── AGENTS.md                   # Workspace documentation for AI agents
    └── <node-id>/                  # Node (document, table, or hybrid)
        ├── index.md                # Document content with YAML front matter
        ├── data.jsonl              # Table records (one JSON per line)
        ├── data.blobs/             # Binary assets (images, files)
        └── <asset-files>           # Asset files stored alongside
```

Every content mutation is a git commit. You can:
- Edit files directly with any text editor
- Sync each workspace to GitHub, GitLab, or any git remote
- Use git history, branches, and merge workflows
- Back up with standard tools

Organization membership is stored in the identity database, not in the file structure.

## FAQ

**Q: Is my data private?**
A: Yes! Everything runs locally. No data is sent anywhere.

**Q: Can I sync across devices?**
A: Configure a git remote in workspace settings—changes push automatically.

**Q: Can I import/export my data?**
A: The data is itself in text form.

## Advanced Users

For developers or advanced setup:
- Review [AGENTS.md](AGENTS.md) for development guidelines

## Building from Source

Development requires Node.js 24 via nvm and pnpm 11.21.0 via Corepack. Clone the repository and run:

```bash
nvm install
nvm use
corepack enable pnpm
make build-all
```

## Comparison

| Feature | Notion | Obsidian | HackMD / ([CodiMD](https://github.com/hackmdio/codimd)) |
| - | - | - | - |
| Organization | ✅ | ✅ | ✅ |
| Workspace | ✅ | ✅ | ✅ |
| Git native storage | ❌ | ❌ | ❌ |
| Export to Git | ❌ | ✅ | ✅ |
| Markdown | ✅ | ✅ | ✅ |
| Self hosted | ❌ | ✅ | ✅ |
| Backend open source | ❌ | ❌ | ✅ |
| Web app first | ✅ | ❌ | ✅ |
| Native table support | ✅ | ✅ | ❌ |

## License

See [LICENSE](LICENSE) file. May change in the future.
