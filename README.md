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
- **NEW**: Clone and run in one command
- **NEW**: Temporary projects with auto-cleanup
- **NEW**: Stop running projects
- **NEW**: Automatic port and command detection
- **NEW**: Fault-tolerant with dependency checking

## Installation

### Quick Install (Recommended)

1. **Check system requirements:**
```bash
make check
```

2. **Install missing dependencies:**
```bash
make install
# Or for development (includes Go):
# make install-dev
```

3. **Build the application:**
```bash
make build
```

### Manual Installation

1. **Install required tools:**
   - **Docker**: Required for running containers
   - **Git**: Required for cloning repositories
   - **Go**: Optional, only needed for development

2. **Install dependencies:**
```bash
# Using the install script
./install.sh

# Or manually:
# - macOS: brew install docker git
# - Linux: sudo apt install docker.io git
```

3. **Build the CLI tool:**
```bash
cd cmd/sandbox
go build -o sandbox .
```

4. **Test installation:**
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

### Clone and Run (NEW)
```bash
# Clone and immediately run in development mode
sandbox clone-dev -r https://github.com/user/repo.git -n my-project
sandbox clone-dev -r https://github.com/user/repo.git -n my-project -v  # Verbose
```

### Temporary Projects (NEW)
```bash
# Clone, run temporarily, and auto-cleanup when stopped
sandbox temporary -r https://github.com/user/repo.git
# Press Ctrl+C to stop and clean up everything
```

### Stop Projects (NEW)
```bash
# Stop all containers for a project (keeps files)
sandbox stop my-project
```

### Detect Project Info (NEW)
```bash
# Show detected ports and commands for a project
sandbox detect --path /path/to/project
sandbox detect  # Current directory
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

### Runtime Requirements (for end users)
- **Docker**: Required for running project containers
- **Git**: Required for cloning repositories

### Development Requirements (for contributors)
- **Go 1.21+**: Only needed to build the application
- **Docker**: As above
- **Git**: As above

### Installation Check
The application automatically checks for required tools on startup and provides helpful error messages with installation instructions if anything is missing.

## Node.js Version Policy

All Node.js projects use the latest Node.js version (22.x) by default to ensure compatibility with modern frameworks like Vite, Next.js, and other tools that require recent Node.js versions. This provides the best development experience and avoids version conflicts.

## Architecture

Each project runs in its own Docker container with:
- Isolated filesystem
- Appropriate runtime environment
- Volume mounting for project files
- Port mapping for web servers

Containers are named with the pattern `sandbox-{project}-{operation}`.
