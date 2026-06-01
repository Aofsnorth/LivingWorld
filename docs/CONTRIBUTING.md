# Contributing to LivingWorld

**Welcome! We're excited that you want to contribute.**

## Ways to Contribute

- 🐛 **Bug Reports** — Report bugs or unexpected behavior
- 💡 **Feature Requests** — Suggest new features
- 📖 **Documentation** — Improve docs, add examples
- 🔧 **Code Contributions** — Submit PRs for fixes/features
- 🧪 **Testing** — Test with different client versions

## Development Setup

### Prerequisites

- Go 1.26.1 or later
- Git
- Minecraft Java Edition (26.1+)
- Minecraft Bedrock Edition (26.21+)

### Clone and Build

```bash
# Clone the repository
git clone https://github.com/yourusername/livingworld.git
cd livingworld

# Add upstream (if you forked)
git remote add upstream https://github.com/original/livingworld.git

# Install dependencies
go mod download

# Build
go build -o livingworld ./cmd/server

# Run tests
go test ./...
```

### Running Locally

```bash
# Start the server
./livingworld

# Or with custom config
./livingworld -config path/to/config.yml
```

## Code Style

We follow standard Go conventions:

- Run `go fmt` before committing
- Use meaningful variable names
- Add comments for complex logic
- Keep functions small and focused

### Example

```go
// Bad
func p(x, y, z int) Block {
    c := w.chunks[ChunkPos{x>>4, z>>4}]
    return c.GetBlock(x&15, y, z&15)
}

// Good
// GetBlock retrieves the block at the specified world coordinates.
// Handles chunk loading automatically if the chunk isn't cached.
func (w *World) GetBlock(x, y, z int) Block {
    chunkX, chunkZ := x>>4, z>>4
    chunk := w.LoadChunk(chunkX, chunkZ)
    return chunk.GetBlock(x&15, y, z&15)
}
```

## Project Structure

```
livingworld/
├── cmd/server/       # Application entry point
├── config/           # Configuration
├── docs/             # Documentation
├── internal/         # Internal packages
│   ├── bedrock/      # Bedrock edition code
│   ├── java/         # Java edition code
│   ├── player/       # Player management
│   ├── plugin/       # Plugin system
│   └── world/        # World system
└── third_party/      # Patched dependencies
```

## Branch Strategy

- `main` — Stable releases
- `develop` — Development work
- `feature/*` — New features
- `fix/*` — Bug fixes
- `protocol/*` — Protocol updates

## Making Changes

### Bug Fixes

1. Create a branch: `git checkout -b fix/description`
2. Write the fix
3. Add/update tests
4. Commit with clear message
5. Push and create PR

### New Features

1. Create a branch: `git checkout -b feature/description`
2. Implement feature
3. Add documentation
4. Add tests
5. Commit with clear message
6. Push and create PR

## Pull Request Guidelines

### Title Format

```
[Type] Short description

Types: Add, Fix, Update, Remove, Refactor, Document, Test
```

### Examples

- `[Add] Java 1.21 protocol support`
- `[Fix] Chunk loading on Bedrock`
- `[Update] go-mc to latest version`
- `[Document] Plugin API examples`

### PR Description Template

```markdown
## Summary
Brief description of changes.

## Motivation
Why is this change needed?

## Testing
How was this tested?

## Screenshots (if applicable)
[Any visual changes]

## Checklist
- [ ] Code follows style guidelines
- [ ] Tests added/updated
- [ ] Documentation updated
- [ ] No breaking changes (or documented)
```

## Reporting Bugs

### Include

- LivingWorld version
- Go version
- Minecraft versions
- Steps to reproduce
- Expected vs actual behavior
- Server logs (if relevant)

### Template

```markdown
**Description**
[Clear description]

**Steps to Reproduce**
1. [Step 1]
2. [Step 2]
3. [Step 3]

**Expected Behavior**
[What should happen]

**Actual Behavior**
[What actually happens]

**Version Info**
- LivingWorld: [version]
- Go: [version]
- Java Client: [version]
- Bedrock Client: [version]

**Logs**
```
[paste logs]
```
```

## Feature Requests

### Include

- Use case
- Proposed solution
- Alternatives considered
- Priority (1-5)

## Questions?

- Open an issue for bugs/features
- Join discussions in PRs
- Check existing issues first

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
