# mddb - Markdown Document & Database System

A Notion-like document and database system for managing your information. Store everything locally as files, no external services required.

## What is mddb?

mddb lets you:
- 📝 **Create and edit documents** - Write in markdown with live preview
- 📊 **Build databases** - Store structured data with typed columns
- 🗂️ **Organize information** - Create folders and nested structures
- ⚡ **Auto-save** - Changes save automatically as you work
- 📁 **Keep it local** - All data stored as files on your computer

Perfect for personal wikis, knowledge bases, project management, and data collection.

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
3. **Create a database** - Switch to Databases tab and create a new one
4. **Add records** - Click the + button in tables to add rows

## Features

### Documents
- Full markdown support with live preview
- Auto-save every 2 seconds while you type
- Organize pages in folders
- Simple and distraction-free editor

### Databases
- Define custom columns with different types:
  - Text, numbers, dates
  - Single and multi-select dropdowns
  - Checkboxes
- Edit records inline in table view
- Add/delete rows easily
- All data stays local

### Storage
- Everything stored as files on your computer
- Easy to backup - just copy the `data/` folder
- Version control friendly (Git-compatible)
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

All your data is stored in simple, human-readable files:

```
data/
└── pages/
    ├── my-page.md              # A document
    ├── tasks.db.json           # Database schema
    ├── tasks.db.jsonl          # Database records
    ├── project/
    │   ├── notes.md
    │   └── contacts.db.json
    └── photo.png               # Attachments
```

You can:
- Edit `.md` files in any text editor
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

**Q: What if I need help?**
A: Check the [Quick Start guide](docs/QUICKSTART.md) or review [the full documentation](docs/PLAN.md).

## Advanced Users

For developers or advanced setup:
- See [QUICKSTART.md](docs/QUICKSTART.md) for detailed instructions
- Review [AGENTS.md](AGENTS.md) for development guidelines
- Check [PLAN.md](docs/PLAN.md) for technical architecture

## Building from Source

Clone the repository and run:
```bash
make build
```

The binary is created in the current directory. See [AGENTS.md](AGENTS.md) for full development setup.

## License

See [LICENSE](LICENSE) file

---

**mddb** - Keep your information local, organized, and yours.
