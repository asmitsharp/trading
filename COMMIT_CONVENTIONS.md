# Commit Message Conventions

This project follows [Conventional Commits](https://www.conventionalcommits.org/) specification.

## Format

```
<type>(<scope>): <subject>

<body>

<footer>
```

## Types

- **feat**: A new feature
- **fix**: A bug fix
- **docs**: Documentation only changes
- **style**: Changes that do not affect the meaning of the code (white-space, formatting, etc)
- **refactor**: A code change that neither fixes a bug nor adds a feature
- **perf**: A code change that improves performance
- **test**: Adding missing tests or correcting existing tests
- **build**: Changes that affect the build system or external dependencies
- **ci**: Changes to CI configuration files and scripts
- **chore**: Other changes that don't modify src or test files
- **revert**: Reverts a previous commit

## Scope (optional)

The scope should be the name of the module affected (e.g., `db`, `api`, `seed`, `migration`)

## Examples

### Feature
```
feat(api): add token metadata endpoint

- Added new endpoint for fetching token metadata
- Integrated with PostgreSQL for token information
- Added proper error handling and validation
```

### Fix
```
fix(db): resolve connection pool exhaustion

Increased max open connections from 10 to 25
to handle concurrent requests properly
```

### Refactor
```
refactor(migration): switch from UUID to serial ID

- Changed tokens table primary key from UUID to SERIAL
- Removed uuid-ossp extension dependency
- Updated related migration files
```

### Build/Infrastructure
```
build(seed): add token seeding utility

- Created cmd/seed for loading tokens from JSON
- Added Makefile targets for database setup
- Updated documentation with seeding instructions
```

## Setup

1. Install dependencies:
```bash
npm install
```

2. Commits will now be automatically validated against these conventions.

## Commit Message for Current Changes

For the changes we just made, here's the appropriate commit message:

```
feat(seed): add token data seeding utility and improve database setup

- Implement token seeder to load cryptocurrency data from JSON
- Update tokens table to use SERIAL ID instead of UUID
- Add Makefile targets for migrations and seeding
- Configure commitlint for conventional commits
- Update documentation with database setup instructions

BREAKING CHANGE: tokens table now uses integer IDs instead of UUIDs
```