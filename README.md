# mddb - Markdown Document & Table System

A Notion-like document and table system for managing your information. Store everything locally as files, no external services required.

[![codecov](https://codecov.io/gh/maruel/mddb/graph/badge.svg?token=Q4SYDP0B00)](https://codecov.io/gh/maruel/mddb)

Why?

<img src="https://github.com/user-attachments/assets/263f10ac-8f5f-484f-a1f8-415d49896de8" style="width: 560px;" />

https://x.com/ivanhzhao/status/2003510041010405790

\-\- Ivan Zhao, Notion CEO

<img src="https://github.com/user-attachments/assets/3e3bbf18-9ffa-4669-9f43-b7e9828e41ef" style="width: 560px;" />

https://x.com/anothercohen/status/2014136100894199968


## What is mddb?

mddb lets you:
- 📝 **Create and edit documents** - Write in markdown with live preview
- 📊 **Build tables** - Store structured data with typed columns
- 🗂️ **Organize information** - Create folders and nested structures
- ⚡ **Auto-save** - Changes save automatically as you work
- 📁 **Keep it local** - All data stored as files on your computer

Perfect for personal wikis, knowledge bases, project management, and data collection.

> **Note**: mddb uses "Table" for what Notion calls a "Database" (a collection of records with columns). Both systems offer the same functionality—mddb just uses clearer terminology to distinguish data structures from the underlying storage system.

## Getting Started

### Installation

1. Download the latest release (or build from source)
2. Run the application:
   ```
   ./mddb
   ```
3. Open http://localhost:8080 in your browser

That's it! All your data is stored in a `data/` folder.

### First Steps

1. **Create a page** - Click "New Page" in the sidebar
2. **Write markdown** - Edit in the left pane, see preview on the right
3. **Create a table** - Switch to Tables tab and create a new one
4. **Add records** - Click the + button in tables to add rows

## Features

### Documents
- Full markdown support with live preview
- Auto-save every 2 seconds while you type
- Organize pages in folders
- Simple and distraction-free editor

### Tables
- Define custom columns with different types:
  - Text, numbers, dates
  - Single and multi-select dropdowns
  - Checkboxes
- Edit records inline in table view
- Add/delete rows easily
- All data stays local

### Search
- Full-text search across documents and tables
- Fast, relevant results
- Search by title, content, or record fields

### Storage
- Everything stored as files on your computer
- **Automatic Git Versioning**: Every change is committed to a local git repo in `data/`
- Easy to backup - just copy the `data/` folder
- No account or internet connection required

## Configuration

When running mddb, you can customize:

```
./mddb -port 8080 -data-dir ./data -log-level info
```

- `-port` - Server port (default: 8080)
- `-data-dir` - Where to store data (default: ./data)
- `-log-level` - Logging verbosity: debug, info, warn, error

## File Structure

All your data is stored in simple, human-readable directories:

```
data/
└── pages/
    ├── 1/                      # First page (document)
    │   ├── index.md
    │   └── favicon.ico
    ├── 2/                      # Second page (table)
    │   ├── index.md
    │   ├── metadata.json       # Table schema
    │   ├── data.jsonl          # Table records
    │   └── favicon.png
    ├── 3/                      # Third page
    │   ├── index.md
    │   ├── photo.png           # Page assets
    │   └── diagram.svg
    └── 4/subfolder/5/          # Nested pages
        ├── index.md
        └── favicon.ico
```

You can:
- Edit `index.md` files in any text editor
- Add images/assets directly in each page's directory
- Back them up with standard tools
- Share them via Git or cloud storage

## FAQ

**Q: Is my data private?**
A: Yes! Everything runs locally. No data is sent anywhere.

**Q: Can I sync across devices?**
A: Put the `data/` folder in Dropbox, Google Drive, or Git to sync.

**Q: Can multiple people use it?**
A: Not simultaneously - designed for single-user or small team with manual sync.

**Q: Can I import/export my data?**
A: Yes! Since everything is markdown and JSON, you can import/export easily.

## Advanced Users

For developers or advanced setup:
- Review [AGENTS.md](AGENTS.md) for development guidelines
- Check [the documentation index](docs/README.md) for more details

## Building from Source

Clone the repository and run:
```bash
make build-all
```

This builds the SolidJS frontend and embeds it in the Go binary using `go:embed`. The result is a single, self-contained executable with no external dependencies.

For detailed information on the technical architecture, embedded build process, and storage model, see [TECHNICAL.md](docs/TECHNICAL.md).

See [AGENTS.md](AGENTS.md) for full development setup and [API.md](docs/API.md) for API reference.

## License

See [LICENSE](LICENSE) file

---

**mddb** - Keep your information local, organized, and yours.
