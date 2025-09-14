package main

import (
	"path/filepath"
	"strings"
)

// ProjectHandler interface for handling different project types
type ProjectHandler interface {
	Detect(projectPath string) bool
	GetDevConfig(projectPath string) (image string, cmd []string)
	GetBuildConfig(projectPath string) (image string, cmd []string)
	GetAssetConfig(projectPath string) (image string, cmd []string)
	GetDevPorts(projectPath string) (hostPort, containerPort string)
	GetAssetsDirectory(projectPath string) string
	GetDependencies(projectPath string) []string
	GetScripts(projectPath string) map[string]string
	GetDescription(projectPath string) string
	GetVersion(projectPath string) string
}

// Handler registry
var handlers = make(map[ProjectType]ProjectHandler)

// RegisterHandler registers a project handler
func RegisterHandler(projectType ProjectType, handler ProjectHandler) {
	handlers[projectType] = handler
}

// GetHandler returns the appropriate handler for a project type
func GetHandler(projectType ProjectType) ProjectHandler {
	return handlers[projectType]
}

// BaseHandler provides common functionality
type BaseHandler struct{}

func (h *BaseHandler) GetDevPorts(projectPath string) (string, string) {
	return "", ""
}

func (h *BaseHandler) GetAssetsDirectory(projectPath string) string {
	return filepath.Join(projectPath, "dist")
}

func (h *BaseHandler) GetDescription(projectPath string) string {
	return ""
}

func (h *BaseHandler) GetVersion(projectPath string) string {
	return ""
}

// NodeJSHandler handles Node.js projects
type NodeJSHandler struct {
	BaseHandler
	isPnpm bool
}

func (h *NodeJSHandler) Detect(projectPath string) bool {
	if fileExists(filepath.Join(projectPath, "package.json")) {
		// Check for pnpm
		h.isPnpm = fileExists(filepath.Join(projectPath, "pnpm-lock.yaml"))
		return true
	}
	return false
}

func (h *NodeJSHandler) GetDevConfig(projectPath string) (string, []string) {
	if h.isPnpm {
		return "node:22", []string{"sh", "-c", "npm install -g pnpm && pnpm install && pnpm run dev --host 0.0.0.0"}
	}
	return "node:22", []string{"sh", "-c", "npm install && npm run dev -- --host 0.0.0.0"}
}

func (h *NodeJSHandler) GetBuildConfig(projectPath string) (string, []string) {
	if h.isPnpm {
		return "node:22", []string{"sh", "-c", "npm install -g pnpm && pnpm install && pnpm run build"}
	}
	return "node:22", []string{"sh", "-c", "npm install && npm run build"}
}

func (h *NodeJSHandler) GetAssetConfig(projectPath string) (string, []string) {
	if h.isPnpm {
		return "node:22", []string{"sh", "-c", "npm install -g pnpm && pnpm install && pnpm run build"}
	}
	return "node:22", []string{"sh", "-c", "npm install && npm run build"}
}

func (h *NodeJSHandler) GetDevPorts(projectPath string) (string, string) {
	if port := detectNodeJSPort(projectPath); port != "" {
		return port, port
	}
	return "3000", "3000"
}

func (h *NodeJSHandler) GetDependencies(projectPath string) []string {
	packagePath := filepath.Join(projectPath, "package.json")
	content := readFileContent(packagePath)
	if content == "" {
		return nil
	}

	var deps []string
	depsStart := strings.Index(content, `"dependencies"`)
	if depsStart != -1 {
		depsEnd := strings.Index(content[depsStart:], "},")
		if depsEnd == -1 {
			depsEnd = strings.Index(content[depsStart:], "}")
		}
		if depsEnd != -1 {
			depsSection := content[depsStart : depsStart+depsEnd+1]
			lines := strings.Split(depsSection, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.Contains(line, `"`) && strings.Contains(line, ":") {
					parts := strings.Split(line, `"`)
					if len(parts) >= 2 {
						packageName := strings.TrimSpace(parts[1])
						if packageName != "dependencies" && packageName != "" {
							deps = append(deps, packageName)
						}
					}
				}
			}
		}
	}

	if len(deps) > 10 {
		deps = deps[:10]
	}
	return deps
}

func (h *NodeJSHandler) GetScripts(projectPath string) map[string]string {
	packagePath := filepath.Join(projectPath, "package.json")
	content := readFileContent(packagePath)
	if content != "" {
		return extractScriptsFromPackageJSON(content)
	}
	return nil
}

func (h *NodeJSHandler) GetDescription(projectPath string) string {
	packagePath := filepath.Join(projectPath, "package.json")
	content := readFileContent(packagePath)
	if content != "" {
		descStart := strings.Index(content, `"description"`)
		if descStart != -1 {
			descStart = strings.Index(content[descStart:], `"`)
			if descStart != -1 {
				descStart += strings.Index(content, `"description"`) + 15
				descEnd := strings.Index(content[descStart:], `"`)
				if descEnd != -1 {
					return strings.TrimSpace(content[descStart : descStart+descEnd])
				}
			}
		}
	}
	return ""
}

// GoHandler handles Go projects
type GoHandler struct {
	BaseHandler
}

func (h *GoHandler) Detect(projectPath string) bool {
	return fileExists(filepath.Join(projectPath, "go.mod"))
}

func (h *GoHandler) GetDevConfig(projectPath string) (string, []string) {
	goVersion := h.GetVersion(projectPath)
	if goVersion != "" {
		parts := strings.Split(goVersion, ".")
		if len(parts) >= 2 {
			major, minor := parts[0], parts[1]
			imageVersion := major + "." + minor
			return "golang:" + imageVersion, []string{"go", "run", "."}
		}
	}
	return "golang:1.21", []string{"go", "run", "."}
}

func (h *GoHandler) GetBuildConfig(projectPath string) (string, []string) {
	goVersion := h.GetVersion(projectPath)
	if goVersion != "" {
		parts := strings.Split(goVersion, ".")
		if len(parts) >= 2 {
			major, minor := parts[0], parts[1]
			imageVersion := major + "." + minor
			return "golang:" + imageVersion, []string{"go", "build", "."}
		}
	}
	return "golang:1.21", []string{"go", "build", "."}
}

func (h *GoHandler) GetAssetConfig(projectPath string) (string, []string) {
	goVersion := h.GetVersion(projectPath)
	if goVersion != "" {
		parts := strings.Split(goVersion, ".")
		if len(parts) >= 2 {
			major, minor := parts[0], parts[1]
			imageVersion := major + "." + minor

			// Check if the version is too new, use latest available
			if major == "1" && (minor == "25" || minor == "24" || minor == "23") {
				imageVersion = "1.21" // Use stable version that should work
			}

			return "golang:" + imageVersion, []string{"echo", "Asset generation not typically needed for Go projects"}
		}
	}
	return "golang:1.21", []string{"echo", "Asset generation not typically needed for Go projects"}
}

func (h *GoHandler) GetDevPorts(projectPath string) (string, string) {
	goPorts := detectGoPorts(projectPath)
	if len(goPorts) > 0 {
		port := goPorts[0]
		return port, port
	}
	return "8080", "8080"
}

func (h *GoHandler) GetDependencies(projectPath string) []string {
	goModPath := filepath.Join(projectPath, "go.mod")
	content := readFileContent(goModPath)
	if content == "" {
		return nil
	}

	var deps []string
	lines := strings.Split(content, "\n")
	inRequireBlock := false
	for _, line := range lines {
		originalLine := line
		line = strings.TrimSpace(line)

		// Handle single-line require statements
		if strings.HasPrefix(line, "require ") && !strings.Contains(line, "(") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				dep := parts[1]
				deps = append(deps, dep)
			}
			continue
		}

		// Handle require block start
		if strings.HasPrefix(line, "require (") {
			inRequireBlock = true
			continue
		}

		// Handle require block end
		if inRequireBlock && line == ")" {
			inRequireBlock = false
			continue
		}

		// Parse dependencies within require block
		if inRequireBlock && line != "" && !strings.HasPrefix(line, "//") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				dep := parts[0]
				if !strings.Contains(originalLine, "// indirect") {
					deps = append(deps, dep)
				}
			}
		}
	}

	if len(deps) > 10 {
		deps = deps[:10]
	}
	return deps
}

func (h *GoHandler) GetScripts(projectPath string) map[string]string {
	return map[string]string{
		"run":   "go run .",
		"build": "go build .",
		"test":  "go test ./...",
	}
}

func (h *GoHandler) GetVersion(projectPath string) string {
	return getGoVersion(projectPath)
}

// PythonHandler handles Python projects
type PythonHandler struct {
	BaseHandler
}

func (h *PythonHandler) Detect(projectPath string) bool {
	return fileExists(filepath.Join(projectPath, "requirements.txt")) ||
		fileExists(filepath.Join(projectPath, "setup.py")) ||
		fileExists(filepath.Join(projectPath, "pyproject.toml"))
}

func (h *PythonHandler) GetDevConfig(projectPath string) (string, []string) {
	return "python:3.11", []string{"sh", "-c", "pip install -r requirements.txt && python app.py"}
}

func (h *PythonHandler) GetBuildConfig(projectPath string) (string, []string) {
	return "python:3.11", []string{"python", "setup.py", "build"}
}

func (h *PythonHandler) GetAssetConfig(projectPath string) (string, []string) {
	return "python:3.11", []string{"echo", "Asset generation not defined for Python projects"}
}

func (h *PythonHandler) GetDevPorts(projectPath string) (string, string) {
	return "5000", "5000"
}

func (h *PythonHandler) GetScripts(projectPath string) map[string]string {
	return map[string]string{
		"run": "python app.py",
	}
}

func (h *PythonHandler) GetDependencies(projectPath string) []string {
	reqPath := filepath.Join(projectPath, "requirements.txt")
	content := readFileContent(reqPath)
	if content == "" {
		return nil
	}

	var deps []string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			// Extract package name (before ==, >=, etc.)
			parts := strings.Split(line, "==")
			if len(parts) > 0 {
				parts = strings.Split(parts[0], ">=")
				if len(parts) > 0 {
					parts = strings.Split(parts[0], "<=")
					if len(parts) > 0 {
						packageName := strings.TrimSpace(parts[0])
						if packageName != "" {
							deps = append(deps, packageName)
						}
					}
				}
			}
		}
	}

	if len(deps) > 10 {
		deps = deps[:10]
	}
	return deps
}

// PHPHandler handles PHP projects
type PHPHandler struct {
	BaseHandler
}

func (h *PHPHandler) Detect(projectPath string) bool {
	return fileExists(filepath.Join(projectPath, "composer.json"))
}

func (h *PHPHandler) GetDevConfig(projectPath string) (string, []string) {
	return "php:8.2-apache", []string{"apache2-foreground"}
}

func (h *PHPHandler) GetBuildConfig(projectPath string) (string, []string) {
	return "php:8.2-cli", []string{"composer", "install", "--no-dev", "--optimize-autoloader"}
}

func (h *PHPHandler) GetAssetConfig(projectPath string) (string, []string) {
	return "php:8.2-cli", []string{"echo", "Asset generation not defined for PHP projects"}
}

func (h *PHPHandler) GetDevPorts(projectPath string) (string, string) {
	return "80", "80"
}

func (h *PHPHandler) GetDependencies(projectPath string) []string {
	composerPath := filepath.Join(projectPath, "composer.json")
	content := readFileContent(composerPath)
	if content == "" {
		return nil
	}

	var deps []string
	requireStart := strings.Index(content, `"require"`)
	if requireStart != -1 {
		requireEnd := strings.Index(content[requireStart:], "},")
		if requireEnd == -1 {
			requireEnd = strings.Index(content[requireStart:], "}")
		}
		if requireEnd != -1 {
			requireSection := content[requireStart : requireStart+requireEnd+1]
			lines := strings.Split(requireSection, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.Contains(line, `"`) && strings.Contains(line, ":") {
					parts := strings.Split(line, `"`)
					if len(parts) >= 2 {
						packageName := strings.TrimSpace(parts[1])
						if packageName != "require" && packageName != "" && !strings.Contains(packageName, "php") {
							deps = append(deps, packageName)
						}
					}
				}
			}
		}
	}

	if len(deps) > 10 {
		deps = deps[:10]
	}
	return deps
}

func (h *PHPHandler) GetScripts(projectPath string) map[string]string {
	return map[string]string{
		"serve": "php -S localhost:8000",
	}
}

// LaravelHandler handles Laravel projects
type LaravelHandler struct {
	BaseHandler
}

func (h *LaravelHandler) Detect(projectPath string) bool {
	if fileExists(filepath.Join(projectPath, "composer.json")) {
		composerContent := readFileContent(filepath.Join(projectPath, "composer.json"))
		return strings.Contains(composerContent, "laravel/framework")
	}
	return false
}

func (h *LaravelHandler) GetDevConfig(projectPath string) (string, []string) {
	return "php:8.2-apache", []string{"sh", "-c", "composer install && php artisan serve --host=0.0.0.0 --port=8000"}
}

func (h *LaravelHandler) GetBuildConfig(projectPath string) (string, []string) {
	return "php:8.2-cli", []string{"sh", "-c", "composer install --no-dev --optimize-autoloader && php artisan config:cache && php artisan route:cache && php artisan view:cache"}
}

func (h *LaravelHandler) GetAssetConfig(projectPath string) (string, []string) {
	return "php:8.2-cli", []string{"sh", "-c", "composer install && php artisan vite:build"}
}

func (h *LaravelHandler) GetDevPorts(projectPath string) (string, string) {
	return "8000", "8000"
}

func (h *LaravelHandler) GetDependencies(projectPath string) []string {
	composerPath := filepath.Join(projectPath, "composer.json")
	content := readFileContent(composerPath)
	if content == "" {
		return nil
	}

	var deps []string
	requireStart := strings.Index(content, `"require"`)
	if requireStart != -1 {
		requireEnd := strings.Index(content[requireStart:], "},")
		if requireEnd == -1 {
			requireEnd = strings.Index(content[requireStart:], "}")
		}
		if requireEnd != -1 {
			requireSection := content[requireStart : requireStart+requireEnd+1]
			lines := strings.Split(requireSection, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.Contains(line, `"`) && strings.Contains(line, ":") {
					parts := strings.Split(line, `"`)
					if len(parts) >= 2 {
						packageName := strings.TrimSpace(parts[1])
						if packageName != "require" && packageName != "" && !strings.Contains(packageName, "php") {
							deps = append(deps, packageName)
						}
					}
				}
			}
		}
	}

	if len(deps) > 10 {
		deps = deps[:10]
	}
	return deps
}

func (h *LaravelHandler) GetScripts(projectPath string) map[string]string {
	return map[string]string{
		"serve":   "php artisan serve",
		"migrate": "php artisan migrate",
	}
}

// CodeIgniterHandler handles CodeIgniter projects
type CodeIgniterHandler struct {
	BaseHandler
}

func (h *CodeIgniterHandler) Detect(projectPath string) bool {
	return fileExists(filepath.Join(projectPath, "system")) &&
		fileExists(filepath.Join(projectPath, "application"))
}

func (h *CodeIgniterHandler) GetDevConfig(projectPath string) (string, []string) {
	return "php:8.2-apache", []string{"sh", "-c", "composer install && apache2-foreground"}
}

func (h *CodeIgniterHandler) GetBuildConfig(projectPath string) (string, []string) {
	return "php:8.2-cli", []string{"composer", "install", "--no-dev", "--optimize-autoloader"}
}

func (h *CodeIgniterHandler) GetAssetConfig(projectPath string) (string, []string) {
	return "php:8.2-cli", []string{"echo", "Asset generation not defined for CodeIgniter projects"}
}

func (h *CodeIgniterHandler) GetDevPorts(projectPath string) (string, string) {
	return "80", "80"
}

func (h *CodeIgniterHandler) GetDependencies(projectPath string) []string {
	composerPath := filepath.Join(projectPath, "composer.json")
	content := readFileContent(composerPath)
	if content == "" {
		return nil
	}

	var deps []string
	requireStart := strings.Index(content, `"require"`)
	if requireStart != -1 {
		requireEnd := strings.Index(content[requireStart:], "},")
		if requireEnd == -1 {
			requireEnd = strings.Index(content[requireStart:], "}")
		}
		if requireEnd != -1 {
			requireSection := content[requireStart : requireStart+requireEnd+1]
			lines := strings.Split(requireSection, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if strings.Contains(line, `"`) && strings.Contains(line, ":") {
					parts := strings.Split(line, `"`)
					if len(parts) >= 2 {
						packageName := strings.TrimSpace(parts[1])
						if packageName != "require" && packageName != "" && !strings.Contains(packageName, "php") {
							deps = append(deps, packageName)
						}
					}
				}
			}
		}
	}

	if len(deps) > 10 {
		deps = deps[:10]
	}
	return deps
}

func (h *CodeIgniterHandler) GetScripts(projectPath string) map[string]string {
	return map[string]string{
		"serve": "php -S localhost:8000",
	}
}

// RustHandler handles Rust projects
type RustHandler struct {
	BaseHandler
}

func (h *RustHandler) Detect(projectPath string) bool {
	return fileExists(filepath.Join(projectPath, "Cargo.toml"))
}

func (h *RustHandler) GetDevConfig(projectPath string) (string, []string) {
	return "rust:1.70", []string{"cargo", "run"}
}

func (h *RustHandler) GetBuildConfig(projectPath string) (string, []string) {
	return "rust:1.70", []string{"cargo", "build", "--release"}
}

func (h *RustHandler) GetAssetConfig(projectPath string) (string, []string) {
	return "rust:1.70", []string{"echo", "Asset generation not defined for Rust projects"}
}

func (h *RustHandler) GetDevPorts(projectPath string) (string, string) {
	return "8080", "8080"
}

func (h *RustHandler) GetDependencies(projectPath string) []string {
	cargoPath := filepath.Join(projectPath, "Cargo.toml")
	content := readFileContent(cargoPath)
	if content == "" {
		return nil
	}

	var deps []string
	dependenciesStart := strings.Index(content, "[dependencies]")
	if dependenciesStart != -1 {
		// Simple extraction - could be improved with TOML parsing
		lines := strings.Split(content[dependenciesStart:], "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "=") && !strings.HasPrefix(line, "[") {
				parts := strings.Split(line, "=")
				if len(parts) > 0 {
					packageName := strings.TrimSpace(parts[0])
					if packageName != "" {
						deps = append(deps, packageName)
					}
				}
			}
		}
	}

	if len(deps) > 10 {
		deps = deps[:10]
	}
	return deps
}

func (h *RustHandler) GetScripts(projectPath string) map[string]string {
	return map[string]string{
		"run":   "cargo run",
		"build": "cargo build",
		"test":  "cargo test",
	}
}

// JavaHandler handles Java projects
type JavaHandler struct {
	BaseHandler
}

func (h *JavaHandler) Detect(projectPath string) bool {
	return fileExists(filepath.Join(projectPath, "pom.xml")) ||
		fileExists(filepath.Join(projectPath, "build.gradle")) ||
		fileExists(filepath.Join(projectPath, "build.gradle.kts"))
}

func (h *JavaHandler) GetDevConfig(projectPath string) (string, []string) {
	return "openjdk:17", []string{"sh", "-c", "javac *.java && java Main"}
}

func (h *JavaHandler) GetBuildConfig(projectPath string) (string, []string) {
	return "openjdk:17", []string{"javac", "*.java"}
}

func (h *JavaHandler) GetAssetConfig(projectPath string) (string, []string) {
	return "openjdk:17", []string{"echo", "Asset generation not defined for Java projects"}
}

func (h *JavaHandler) GetDevPorts(projectPath string) (string, string) {
	return "8080", "8080"
}

func (h *JavaHandler) GetDependencies(projectPath string) []string {
	// For Maven projects
	pomPath := filepath.Join(projectPath, "pom.xml")
	content := readFileContent(pomPath)
	if content != "" {
		var deps []string
		// Simple XML parsing - could be improved
		lines := strings.Split(content, "\n")
		inDependencies := false
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "<dependencies>") {
				inDependencies = true
				continue
			}
			if strings.Contains(line, "</dependencies>") {
				inDependencies = false
				continue
			}
			if inDependencies && strings.Contains(line, "<artifactId>") {
				parts := strings.Split(line, ">")
				if len(parts) > 1 {
					artifactId := strings.Split(parts[1], "<")[0]
					if artifactId != "" {
						deps = append(deps, artifactId)
					}
				}
			}
		}
		if len(deps) > 10 {
			deps = deps[:10]
		}
		return deps
	}

	// For Gradle projects - simplified
	gradlePath := filepath.Join(projectPath, "build.gradle")
	content = readFileContent(gradlePath)
	if content != "" {
		var deps []string
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "implementation") || strings.Contains(line, "compile") {
				// Simple extraction
				parts := strings.Split(line, "'")
				if len(parts) >= 2 {
					dep := strings.Split(parts[1], ":")
					if len(dep) > 0 {
						deps = append(deps, dep[0])
					}
				}
			}
		}
		if len(deps) > 10 {
			deps = deps[:10]
		}
		return deps
	}

	return nil
}

func (h *JavaHandler) GetScripts(projectPath string) map[string]string {
	return map[string]string{
		"run":   "./gradlew run",
		"build": "./gradlew build",
	}
}

// CSharpHandler handles C# projects
type CSharpHandler struct {
	BaseHandler
}

func (h *CSharpHandler) Detect(projectPath string) bool {
	return fileExists(filepath.Join(projectPath, "*.csproj")) ||
		fileExists(filepath.Join(projectPath, "Program.cs"))
}

func (h *CSharpHandler) GetDevConfig(projectPath string) (string, []string) {
	return "mcr.microsoft.com/dotnet/sdk:7.0", []string{"sh", "-c", "dotnet restore && dotnet run"}
}

func (h *CSharpHandler) GetBuildConfig(projectPath string) (string, []string) {
	return "mcr.microsoft.com/dotnet/sdk:7.0", []string{"dotnet", "publish", "-c", "Release"}
}

func (h *CSharpHandler) GetAssetConfig(projectPath string) (string, []string) {
	return "mcr.microsoft.com/dotnet/sdk:7.0", []string{"echo", "Asset generation not defined for C# projects"}
}

func (h *CSharpHandler) GetDevPorts(projectPath string) (string, string) {
	return "5000", "5000"
}

func (h *CSharpHandler) GetDependencies(projectPath string) []string {
	// For .NET projects - simplified parsing
	csprojPath := filepath.Join(projectPath, "*.csproj")
	content := readFileContent(csprojPath)
	if content != "" {
		var deps []string
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "<PackageReference") {
				// Extract Include attribute
				includeStart := strings.Index(line, `Include="`)
				if includeStart != -1 {
					includeStart += 9
					includeEnd := strings.Index(line[includeStart:], `"`)
					if includeEnd != -1 {
						packageName := line[includeStart : includeStart+includeEnd]
						deps = append(deps, packageName)
					}
				}
			}
		}
		if len(deps) > 10 {
			deps = deps[:10]
		}
		return deps
	}
	return nil
}

func (h *CSharpHandler) GetScripts(projectPath string) map[string]string {
	return map[string]string{
		"run":   "dotnet run",
		"build": "dotnet build",
	}
}

// RubyHandler handles Ruby projects
type RubyHandler struct {
	BaseHandler
}

func (h *RubyHandler) Detect(projectPath string) bool {
	return fileExists(filepath.Join(projectPath, "Gemfile")) ||
		fileExists(filepath.Join(projectPath, "Rakefile"))
}

func (h *RubyHandler) GetDevConfig(projectPath string) (string, []string) {
	return "ruby:3.2", []string{"sh", "-c", "bundle install && ruby app.rb"}
}

func (h *RubyHandler) GetBuildConfig(projectPath string) (string, []string) {
	return "ruby:3.2", []string{"bundle", "exec", "rake", "build"}
}

func (h *RubyHandler) GetAssetConfig(projectPath string) (string, []string) {
	return "ruby:3.2", []string{"echo", "Asset generation not defined for Ruby projects"}
}

func (h *RubyHandler) GetDevPorts(projectPath string) (string, string) {
	return "4567", "4567"
}

func (h *RubyHandler) GetDependencies(projectPath string) []string {
	gemfilePath := filepath.Join(projectPath, "Gemfile")
	content := readFileContent(gemfilePath)
	if content == "" {
		return nil
	}

	var deps []string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "gem ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				gemName := strings.Trim(parts[1], `",`)
				if gemName != "" {
					deps = append(deps, gemName)
				}
			}
		}
	}

	if len(deps) > 10 {
		deps = deps[:10]
	}
	return deps
}

func (h *RubyHandler) GetScripts(projectPath string) map[string]string {
	return map[string]string{
		"server":  "ruby app.rb",
		"console": "irb",
	}
}
