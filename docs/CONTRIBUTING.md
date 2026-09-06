# Contributing Guidelines

**English** | [简体中文](CONTRIBUTING-zh-CN.md)

First, thank you for considering contributions to Context-Keeper! 🎉

We welcome all sorts of contributions: reporting bugs, proposing features, improving documentation, fixing typos, or implementing new features. Whether you're a first-time contributor or a seasoned developer, your involvement helps make Context-Keeper better for everyone.

## Table of Contents

- [Contributing Guidelines](#contributing-guidelines)
  - [Table of Contents](#table-of-contents)
  - [Prerequisite](#prerequisite)
  - [Setup](#setup)
  - [Contribution Option 1 - Reporting an Issue](#contribution-option-1---reporting-an-issue)
    - [How Issues Will Be Handled](#how-issues-will-be-handled)
  - [Contribution Option 2 - Update Documentation](#contribution-option-2---update-documentation)
    - [For Trivial Doc Changes](#for-trivial-doc-changes)
    - [For Non-Trivial Doc Changes](#for-non-trivial-doc-changes)
  - [Contribution Option 3 - Code Changes](#contribution-option-3---code-changes)
    - [For Trivial Changes](#for-trivial-changes)
    - [For Non-Trivial Changes](#for-non-trivial-changes)
    - [Update API Documentation](#update-api-documentation)
  - [Contributor Resources](#contributor-resources)
    - [PR and Commit Guidelines](#pr-and-commit-guidelines)
    - [Understanding Labels](#understanding-labels)
    - [Growth Path for Contributors](#growth-path-for-contributors)
    - [Where Can You Get Support](#where-can-you-get-support)
    - [More Resources](#more-resources)

---

## Prerequisite

### Sign the DCO

For every commit, **you must sign off on the [DCO (Developer Certificate of Origin)](https://developercertificate.org/)**. This confirms you have the right to submit your contribution.

```bash
# Add -s flag to your commit
git commit -s -m "feat: add new feature"
```

### Follow Code of Conduct

This project and everyone participating in it is governed by our [Code of Conduct](CODE_OF_CONDUCT.md). By participating, you are expected to uphold this code. Please report unacceptable behavior to contact@context-keeper.com.

---

## Setup

### Fork and Clone

First, **fork the repository** on GitHub, then clone your fork:

```bash
# 1. Clone your fork
git clone https://github.com/YOUR_USERNAME/context-keeper.git
cd context-keeper

# 2. Add upstream remote
git remote add upstream https://github.com/redleaves/context-keeper.git

# 3. Verify remotes
git remote -v
```

### Install Dependencies

**Prerequisites:**
- **Go 1.21+** (required for backend)
- **Node.js 16+** (required for VSCode extension)
- **Docker** (optional, for running dependencies)
- Basic understanding of:
  - Go programming language
  - LLM and vector databases
  - MCP protocol (helpful but not required)

**Installation:**

```bash
# Install Go dependencies
go mod download

# Install Node.js dependencies (for VSCode extension)
cd cursor-integration
npm install
cd ..

# Copy and configure environment
cp config/env.template config/.env
# Edit config/.env with your settings
```

### Setup Development Environment

**Option 1: Using Docker Compose (Recommended)**

```bash
# Start required services (Vearch, TimescaleDB, Neo4j)
docker-compose up -d

# Verify services are running
docker-compose ps
```

**Option 2: Manual Setup**

You'll need to set up:
- Vector Store (Vearch or DashVector, or any custom vector database based on the interface extension)
- Time-series Database (TimescaleDB)
- Graph Database (Neo4j)

See [deployment guide](README.md#4-deployment--integration) for details.

### Run Tests

```bash
# Run unit tests
go test ./...

# Run integration tests
go test ./tests/integration/...

# Run with coverage
go test -cover ./...
```

### Start Development Server

```bash
# Start HTTP server
./scripts/manage.sh deploy http --port 8088

# Verify server is running
curl http://localhost:8088/health
```

---

## Contribution Option 1 - Reporting an Issue

**For security-related issues, please email contact@context-keeper.com instead.**

Found a bug or have a feature request? Here's how to report it:

### Step 1: Search Existing Issues

Before creating a new issue, **search existing issues** to avoid duplicates:

- [Open Issues](https://github.com/redleaves/context-keeper/issues)
- [Closed Issues](https://github.com/redleaves/context-keeper/issues?q=is%3Aissue+is%3Aclosed)

Useful labels to help you navigate:
- [`good first issue`](https://github.com/redleaves/context-keeper/labels/good%20first%20issue) - Good for newcomers
- [`help wanted`](https://github.com/redleaves/context-keeper/labels/help%20wanted) - Extra attention needed
- [`bug`](https://github.com/redleaves/context-keeper/labels/bug) - Something isn't working
- [`enhancement`](https://github.com/redleaves/context-keeper/labels/enhancement) - New feature or request
- [`documentation`](https://github.com/redleaves/context-keeper/labels/documentation) - Documentation improvements

### Step 2: Create Issue

If no issue exists, create a new one:

**For Bug Reports:**
1. Use the bug report template
2. Include:
   - Clear description of the bug
   - Steps to reproduce
   - Expected vs actual behavior
   - Environment details (OS, Go version, etc.)
   - Minimal reproducible example
   - Relevant logs or screenshots

**For Feature Requests:**
1. Use the feature request template
2. Include:
   - Clear description of the feature
   - Use case and motivation
   - Proposed solution (if any)
   - Alternatives considered
   - Examples or mockups (if applicable)

**For Documentation Issues:**
1. Describe what's missing or unclear
2. Suggest improvements
3. Link to relevant sections

### Step 3: Wait for Triage

A project maintainer will:
1. Review and triage the issue
2. Apply appropriate labels
3. May ask for additional information
4. Decide if the issue will be accepted

**Please be patient** - we manage a high volume of issues. Do not bump the issue unless you have new information to provide.

### How Issues Will Be Handled

Issues will be tagged with one of the following:
- [`duplicate`](https://github.com/redleaves/context-keeper/labels/duplicate) - Issue already exists
- [`wontfix`](https://github.com/redleaves/context-keeper/labels/wontfix) - Won't be worked on (with rationale)
- [`good first issue`](https://github.com/redleaves/context-keeper/labels/good%20first%20issue) - Good for newcomers
- [`help wanted`](https://github.com/redleaves/context-keeper/labels/help%20wanted) - Community contributions welcome

---

## Contribution Option 2 - Update Documentation

Documentation improvements are always welcome! Clear and comprehensive docs help everyone.

### For Trivial Doc Changes

For simple fixes like typos, broken links, or formatting:

1. **Make your changes** directly in your fork
2. **Create a Pull Request** following our [PR guidelines](#pr-and-commit-guidelines)
3. **One of our maintainers** will review and merge it

### For Non-Trivial Doc Changes

For substantial changes like:
- Restructuring documentation
- Adding new guides or tutorials
- Rewriting major sections

**Follow this process:**

1. **Create an issue** describing your proposed changes
2. **Wait for approval** from maintainers
3. **Make your changes** in your fork
4. **Create a Pull Request** with clear description

**Documentation locations:**
- Main README: `README.md`, `README-zh-CN.md`
- Architecture docs: `docs/ARCHITECTURE.md`
- API docs: `docs/api/`
- Developer docs: `docs/dev/`
- Usage guides: `docs/usage/`

---

## Contribution Option 3 - Code Changes

For any code changes, you **need to have an associated issue**: either an existing one or [create a new one](#contribution-option-1---reporting-an-issue).

> ⚠️ **To prevent duplicate work, please claim an issue by commenting that you are working on it.**

### For Trivial Changes

**Examples:** Fixing typos in code comments, minor refactoring, updating dependencies

**Process:**
1. **Claim the issue** by commenting on it
2. **Make your changes** in your fork:
   ```bash
   git checkout -b fix/issue-description
   # Make your changes
   go test ./...
   git add .
   git commit -s -m "fix: description of fix"
   ```
3. **Push and create PR**:
   ```bash
   git push origin fix/issue-description
   # Go to GitHub and create Pull Request
   ```
4. **Follow our [PR guidelines](#pr-and-commit-guidelines)**

### For Non-Trivial Changes

**Examples:** New features, architectural changes, significant refactoring

**Process:**

1. **Claim the issue** by commenting on it

2. **Write a design proposal**:
   - Describe the problem and proposed solution
   - Include:
     - Architecture changes
     - API changes
     - Database schema changes
     - Migration strategy
     - Testing strategy
     - Performance implications

3. **Submit design for review**:
   - Create a new PR with your design document
   - Name it: `[RFC] Your Design Title`
   - Place in: `docs/dev/rfcs/YYYY-MM-DD-design-name.md`

4. **Wait for design approval**:
   - Maintainers will review and provide feedback
   - Iterate on the design until approved
   - Design PR must be merged before implementation

5. **Implement the approved design**:
   ```bash
   git checkout -b feature/your-feature-name
   # Implement your changes
   go test ./...
   git commit -s -m "feat: implement feature"
   ```

6. **We encourage atomic PRs**:
   - Break large changes into smaller sub-tasks
   - Create one PR for each sub-task
   - Each PR should be independently reviewable and mergeable

7. **Create Pull Request** following our guidelines

### Update API Documentation

Context-Keeper exposes APIs through MCP protocol. If you modify any APIs:

**For HTTP APIs:**
1. Update API documentation in `docs/api/`
2. Include request/response examples
3. Update OpenAPI spec if applicable

**For MCP Tools:**
1. Update tool descriptions in code
2. Update MCP manifest: `config/manifests/context-keeper-manifest.json`
3. Test with MCP client:
   ```bash
   # Test your changes
   ./scripts/test-mcp-tools.sh
   ```

**Documentation checklist:**
- [ ] Update API documentation
- [ ] Add request/response examples
- [ ] Update MCP manifest if needed
- [ ] Test with actual MCP client
- [ ] Update integration guides if applicable

---

## Contributor Resources

### PR and Commit Guidelines

**Pull Request Requirements:**
- [ ] PR title follows [Conventional Commits](https://www.conventionalcommits.org/)
- [ ] PR description clearly explains the changes
- [ ] All tests pass (`go test ./...`)
- [ ] Code follows style guidelines (see below)
- [ ] Documentation updated if needed
- [ ] DCO signed off on all commits
- [ ] No merge conflicts with main branch

**Commit Message Format:**

We follow [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): subject

body (optional)

footer (optional)
```

**Types:**
- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation only
- `style`: Code style changes (formatting, missing semicolons, etc.)
- `refactor`: Code refactoring (no functional changes)
- `perf`: Performance improvements
- `test`: Adding or updating tests
- `chore`: Maintenance tasks (dependencies, build, etc.)

**Examples:**

```bash
# Feature
git commit -s -m "feat(retrieval): add multi-dimensional fusion engine

- Implement semantic + temporal + knowledge fusion
- Add LLM-driven ranking strategy
- Include comprehensive test coverage

Closes #123"

# Bug fix
git commit -s -m "fix(session): resolve workspace isolation issue

Sessions from different workspaces could interfere with each other.
Added workspace path validation and isolation checks.

Fixes #456"

# Documentation
git commit -s -m "docs(readme): update quick start guide"

# Chore
git commit -s -m "chore(deps): update go dependencies"
```

**Code Style Guidelines:**

**For Go:**
- Follow [Effective Go](https://golang.org/doc/effective_go)
- Use `gofmt` for formatting
- Follow [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md)
- Run `golangci-lint run` before committing

**For TypeScript/JavaScript:**
- Use ESLint and Prettier
- Follow [Airbnb JavaScript Style Guide](https://github.com/airbnb/javascript)
- Run `npm run lint` before committing

**Testing Requirements:**
- Write unit tests for new functionality
- Aim for >80% code coverage
- Include integration tests for complex features
- Test edge cases and error conditions

### Understanding Labels

We use labels to organize issues and PRs:

**Priority:**
- `priority: high` - Critical issues
- `priority: medium` - Standard priority
- `priority: low` - Nice to have

**Type:**
- `bug` - Something isn't working
- `enhancement` - New feature or improvement
- `documentation` - Documentation improvements
- `question` - Questions or discussions

**Status:**
- `good first issue` - Good for newcomers
- `help wanted` - Extra attention needed
- `in progress` - Someone is working on it
- `needs review` - Waiting for review
- `blocked` - Blocked by other issues

**Area:**
- `area: api` - API related
- `area: storage` - Storage layer
- `area: retrieval` - Retrieval engine
- `area: mcp` - MCP protocol
- `area: integration` - IDE/tool integration

### Growth Path for Contributors

We value and recognize our contributors:

| Level | Criteria | Badge | Privileges |
|-------|----------|-------|------------|
| **Contributor** | 1+ merged PR | ![Contributor](https://img.shields.io/badge/Contributor-green) | Acknowledged in README |
| **Core Contributor** | 5+ merged PRs or significant feature | ![Core](https://img.shields.io/badge/Core%20Contributor-blue) | Can review PRs, triage issues |
| **Maintainer** | Ongoing significant contributions | ![Maintainer](https://img.shields.io/badge/Maintainer-purple) | Merge access, release management |

**How to become a maintainer:**
- Demonstrate sustained high-quality contributions
- Active participation in code reviews
- Help with issue triage and community support
- Show deep understanding of the project
- Maintainers are invited, not applied for

### Where Can You Get Support

**Questions and Discussions:**
- [GitHub Discussions](https://github.com/redleaves/context-keeper/discussions) - For questions and general discussion
- [GitHub Issues](https://github.com/redleaves/context-keeper/issues) - For bugs and feature requests

**Contact:**
- Email: contact@context-keeper.com
- WeChat: **arm386x86**

**Response Times:**
- Issues: 1-3 business days for triage
- PRs: 2-5 business days for initial review
- Security issues: 24 hours

### More Resources

**Learning Resources:**
- [Go Official Documentation](https://golang.org/doc/)
- [Effective Go](https://golang.org/doc/effective_go)
- [MCP Protocol Documentation](https://github.com/modelcontextprotocol)
- [Vector Database Concepts](https://www.pinecone.io/learn/vector-database/)

**Project Resources:**
- [Architecture Design](docs/ARCHITECTURE.md)
- [Project Structure](docs/dev/README_PROJECT_STRUCTURE.md)
- [Deployment Guide](README.md#-deployment--integration)
- [API Reference](docs/api/)

**Development Tools:**
- [golangci-lint](https://golangci-lint.run/) - Go linter
- [Conventional Commits](https://www.conventionalcommits.org/) - Commit message standard
- [GitHub CLI](https://cli.github.com/) - GitHub command line tool

---

## Frequently Asked Questions

**Q: I'm new to open source. Where should I start?**

A: Start with issues labeled [`good first issue`](https://github.com/redleaves/context-keeper/labels/good%20first%20issue). These are specifically chosen to be beginner-friendly. Don't hesitate to ask questions!

**Q: How long will it take to review my PR?**

A: We aim to provide initial feedback within 2-5 business days. Complex PRs may take longer. If you haven't heard back after a week, feel free to politely ping us.

**Q: Can I work on multiple issues at once?**

A: Yes, but we recommend focusing on one at a time, especially when starting out. This helps maintain quality and allows faster iterations.

**Q: My PR has merge conflicts. What should I do?**

A: Rebase your branch on the latest main:
```bash
git fetch upstream
git rebase upstream/main
# Resolve conflicts
git push --force-with-lease origin your-branch
```

**Q: How do I sync my fork with the upstream repository?**

A:
```bash
git checkout main
git fetch upstream
git merge upstream/main
git push origin main
```

**Q: Can I submit breaking changes?**

A: Breaking changes require strong justification and must go through the design proposal process. Discuss with maintainers first.

---

## Thank You!

Thank you for contributing to Context-Keeper! 🎉

Every contribution, no matter how small, helps us build a better intelligent memory system. We look forward to your contributions!

---

**Questions?** Feel free to:
1. Check [GitHub Discussions](https://github.com/redleaves/context-keeper/discussions)
2. Review [existing issues](https://github.com/redleaves/context-keeper/issues)
3. Contact us at contact@context-keeper.com

*Happy Contributing! 💪*
