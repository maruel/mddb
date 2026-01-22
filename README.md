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

> **Note**: mddb uses different terminology than Notion for clarity:
> - **Table** instead of "Database" (a collection of records with columns)
> - **Organization** instead of "Workspace" (a synced collection of pages)
>
> Same functionality, clearer names.

## Getting Started

### Installation

1. Download the latest release (or build from source)
2. Run the application:
   ```
   ./mddb
   ```
3. Open http://localhost:8080 in your browser

That's it! All your data is stored in a `data/` folder.

## Configuration

Run `./mddb -help` and go from there.

## File Structure

All data is stored as plain files in a git repository:

```
data/
└── <org-id>/
    └── pages/
        ├── <page-id>/              # Document
        │   ├── index.md            # Content with YAML front matter
        │   └── photo.png           # Assets stored alongside
        └── <page-id>/              # Table
            ├── metadata.json       # Schema definition
            └── data.jsonl          # Records (one JSON per line)
```

Every mutation is a git commit. You can:
- Edit files directly with any text editor
- Sync to GitHub, GitLab, or any git remote
- Use git history, branches, and merge workflows
- Back up with standard tools

## FAQ

**Q: Is my data private?**
A: Yes! Everything runs locally. No data is sent anywhere.

**Q: Can I sync across devices?**
A: Configure a git remote in organization settings—changes push automatically.

**Q: Can I import/export my data?**
A: The data is itself in text form.

## Advanced Users

For developers or advanced setup:
- Review [AGENTS.md](AGENTS.md) for development guidelines
- Check [the documentation index](docs/README.md) for more details

## Building from Source

Clone the repository and run:
```bash
make build-all
```

## License

See [LICENSE](LICENSE) file
