# mddb Documentation

Welcome to mddb! Here's a guide to all available documentation.

## Getting Started

**New to mddb?** Start here:

1. [QUICKSTART.md](QUICKSTART.md) - Get running in 2 minutes
   - Prerequisites
   - Backend setup
   - Frontend setup
   - Common tasks

2. [README.md](../README.md) - Main project overview
   - Features and capabilities
   - Installation instructions
   - File structure
   - FAQ

3. [EMBEDDED_BUILD.md](EMBEDDED_BUILD.md) - Building and distribution
   - Single binary build process
   - Technical details
   - Development vs Distribution

## Understanding the Project

**Want to understand requirements, architecture, and plan?**

- [REQUIREMENTS.md](REQUIREMENTS.md) - Complete requirements document
  - Functional requirements
  - Non-functional requirements
  - Storage model requirements

- [PLAN.md](PLAN.md) - Implementation roadmap and technical design
  - Overview and principles
  - Data model and storage format
  - API architecture
  - Implementation phases
  - Technical decisions

- [AGENTS.md](../AGENTS.md) - Development guidelines (root level)
  - Project overview and storage model
  - Directory structure
  - Go development patterns
  - Frontend development patterns
  - API conventions
  - Testing practices
  - Git workflow

## Reference

- [LINTERS.md](LINTERS.md) - Code quality standards
- [ASSET_API.md](ASSET_API.md) - Asset management API reference

## Quick Commands

```bash
# Development
make dev              # Start backend server
make frontend-dev     # Start frontend dev server
make build-all        # Build everything

# Testing
make test             # Run all tests
make test-backend     # Run Go tests only

# Building
make build            # Build Go binary
make frontend-build   # Build frontend for production

# Code quality
make lint             # Run all linters
make lint-fix         # Auto-fix all linting issues

# Cleanup
make clean            # Remove binaries and data
```

## Storage Model

Every page—document or database—is a directory with a numeric ID (1, 2, 3, etc.):

```
data/
└── pages/
    ├── 1/                    # Document page
    │   ├── index.md          # Content with YAML front matter
    │   └── favicon.ico       # Optional icon
    ├── 2/                    # Database page
    │   ├── index.md
    │   ├── metadata.json     # Schema definition
    │   ├── data.jsonl        # Records (one per line)
    │   └── favicon.png
    └── 3/subfolder/4/        # Nested organization
        ├── index.md
        └── favicon.ico
```

Benefits:
- **Asset namespace**: Each page owns its assets (images, files, etc.)
- **Clarity**: Every page is a directory—no ambiguity
- **Scalability**: Numeric IDs avoid collisions
- **Organization**: Natural hierarchical structure
- **Version control**: Directories are git-friendly

## Documentation Structure

```
docs/
├── INDEX.md              # You are here - Documentation index
├── REQUIREMENTS.md       # What the system must do
├── PLAN.md               # How we'll build it (roadmap & design)
├── QUICKSTART.md         # Get started in 2 minutes
├── EMBEDDED_BUILD.md     # Single binary build guide
├── ASSET_API.md          # Asset API reference
└── LINTERS.md            # Code quality standards
```
