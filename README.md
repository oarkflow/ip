# Sandbox Project Manager

A CLI tool for managing projects in isolated Docker containers.

## Features

- Clone projects from Git repositories
- Run projects in development mode with appropriate containers
- Build projects
- Generate assets
- Serve built assets
- List and remove projects
- Automatic project type detection (Node.js, Go, Python, Rust)

## Installation

1. Build the CLI tool:
```bash
cd cmd/sandbox
go build -o sandbox .
```

2. Move to your PATH or use directly:
```bash
./cmd/sandbox/sandbox --help
```

## Usage

### Clone a project
```bash
# Basic clone (auto-generates directory name)
sandbox clone -r https://github.com/user/repo.git

# Clone with custom name
sandbox clone -r https://github.com/user/repo.git -n my-project

# Clone with custom directory (takes precedence over -n)
sandbox clone -r https://github.com/user/repo.git -d custom-dir
sandbox clone -r https://github.com/user/repo.git --dir my-custom-folder
```

### Run in development mode
```bash
sandbox dev my-project
sandbox dev my-project -v  # Verbose output
```

### Build the project
```bash
sandbox build my-project
sandbox build my-project -v  # Verbose output
```

### Generate assets
```bash
sandbox generate-assets my-project
```

### Serve built assets
```bash
# Serve built assets from dist/build directory (run 'sandbox build' first)
sandbox serve my-project
sandbox serve my-project -p 3000
```

### List projects
```bash
sandbox list
```

### Remove a project
```bash
sandbox remove my-project
```

## Project Types Supported

### JavaScript/TypeScript Frameworks
- **Node.js**: Detected by `package.json`
- **React**: Detected by `package.json` containing "react" (but not Next.js)
- **Next.js**: Detected by `package.json` containing "next" or `next.config.js`
- **Vite**: Detected by `package.json` containing "vite" or `vite.config.js`
- **pnpm**: Detected by `pnpm-lock.yaml` or `pnpm-workspace.yaml`

### Backend Languages
- **Go**: Detected by `go.mod`
- **Python**: Detected by `requirements.txt`, `setup.py`, or `pyproject.toml`
- **Rust**: Detected by `Cargo.toml`
- **PHP**: Detected by `composer.json`
- **Laravel**: Detected by `composer.json` containing "laravel/framework"
- **CodeIgniter**: Detected by `system/` and `application/` directories
- **Java**: Detected by `pom.xml` or `build.gradle`
- **C#**: Detected by `.csproj` files or `Program.cs`
- **Ruby**: Detected by `Gemfile` or `Rakefile`

## Requirements

- Go 1.21+
- Docker
- Git

## Node.js Version Policy

All Node.js projects use the latest Node.js version (22.x) by default to ensure compatibility with modern frameworks like Vite, Next.js, and other tools that require recent Node.js versions. This provides the best development experience and avoids version conflicts.

## Architecture

Each project runs in its own Docker container with:
- Isolated filesystem
- Appropriate runtime environment
- Volume mounting for project files
- Port mapping for web servers

Containers are named with the pattern `sandbox-{project}-{operation}`.
