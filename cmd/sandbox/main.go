package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/urfave/cli/v2"
	"golang.org/x/mod/modfile"
)

// ProjectType represents the type of project
type ProjectType string

const (
	NodeJS         ProjectType = "nodejs"
	NodeJSVite     ProjectType = "nodejs-vite"
	NodeJSReact    ProjectType = "nodejs-react"
	NodeJSNext     ProjectType = "nodejs-next"
	NodeJSPnpm     ProjectType = "nodejs-pnpm"
	Go             ProjectType = "go"
	Python         ProjectType = "python"
	PHP            ProjectType = "php"
	PHPLaravel     ProjectType = "php-laravel"
	PHPCodeIgniter ProjectType = "php-codeigniter"
	Rust           ProjectType = "rust"
	Java           ProjectType = "java"
	CSharp         ProjectType = "csharp"
	Ruby           ProjectType = "ruby"
	Unknown        ProjectType = "unknown"
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
			imageVersion := parts[0] + "." + parts[1]
			return "golang:" + imageVersion, []string{"go", "run", "."}
		}
	}
	return "golang:1.22", []string{"go", "run", "."}
}

func (h *GoHandler) GetBuildConfig(projectPath string) (string, []string) {
	return "golang:1.21", []string{"go", "build", "."}
}

func (h *GoHandler) GetAssetConfig(projectPath string) (string, []string) {
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
	goModPath := filepath.Join(projectPath, "go.mod")
	content := readFileContent(goModPath)
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "go ") {
			return strings.TrimPrefix(line, "go ")
		}
	}
	return ""
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

func checkRequiredTools() error {
	var missing []string

	// Check for Docker
	if _, err := exec.LookPath("docker"); err != nil {
		missing = append(missing, "docker")
	}

	// Check for Git
	if _, err := exec.LookPath("git"); err != nil {
		missing = append(missing, "git")
	}

	if len(missing) > 0 {
		return fmt.Errorf("  - %s", strings.Join(missing, "\n  - "))
	}

	return nil
}

func init() {
	// Register project handlers
	RegisterHandler(NodeJS, &NodeJSHandler{})
	RegisterHandler(NodeJSVite, &NodeJSHandler{isPnpm: false})
	RegisterHandler(NodeJSReact, &NodeJSHandler{isPnpm: false})
	RegisterHandler(NodeJSNext, &NodeJSHandler{isPnpm: false})
	RegisterHandler(NodeJSPnpm, &NodeJSHandler{isPnpm: true})
	RegisterHandler(Go, &GoHandler{})
	RegisterHandler(Python, &PythonHandler{})
	RegisterHandler(PHP, &PHPHandler{})
	RegisterHandler(PHPLaravel, &LaravelHandler{})
	RegisterHandler(PHPCodeIgniter, &CodeIgniterHandler{})
	RegisterHandler(Rust, &RustHandler{})
	RegisterHandler(Java, &JavaHandler{})
	RegisterHandler(CSharp, &CSharpHandler{})
	RegisterHandler(Ruby, &RubyHandler{})
}

func main() {
	// Check for required tools
	if err := checkRequiredTools(); err != nil {
		fmt.Printf("❌ Missing required tools:\n%s\n", err)
		fmt.Println("Please run 'make install' or './install.sh' to install missing tools.")
		os.Exit(1)
	}

	app := &cli.App{
		Name:  "sandbox",
		Usage: "A project manager with sandbox containers",
		Commands: []*cli.Command{
			{
				Name:  "clone",
				Usage: "Clone a project from git repository",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "repo",
						Aliases:  []string{"r"},
						Usage:    "Git repository URL",
						Required: true,
					},
					&cli.StringFlag{
						Name:    "name",
						Aliases: []string{"n"},
						Usage:   "Project name and directory (optional, defaults to repo name)",
					},
					&cli.StringFlag{
						Name:    "dir",
						Aliases: []string{"d"},
						Usage:   "Custom directory name (optional, overrides name)",
					},
				},
				Action: cloneProject,
			},
			{
				Name:  "dev",
				Usage: "Run project in development mode",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "verbose",
						Aliases: []string{"v"},
						Usage:   "Show verbose output",
					},
				},
				Action: runDev,
			},
			{
				Name:  "build",
				Usage: "Build the project",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "verbose",
						Aliases: []string{"v"},
						Usage:   "Show verbose output",
					},
				},
				Action: buildProject,
			},
			{
				Name:   "generate-assets",
				Usage:  "Generate project assets",
				Action: generateAssets,
			},
			{
				Name:  "serve",
				Usage: "Serve the built assets",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "port",
						Aliases: []string{"p"},
						Usage:   "Port to serve assets on",
						Value:   "8080",
					},
				},
				Action: serveAssets,
			},
			{
				Name:   "list",
				Usage:  "List all projects",
				Action: listProjects,
			},
			{
				Name:   "remove",
				Usage:  "Remove a project",
				Action: removeProject,
			},
			{
				Name:  "detect",
				Usage: "Detect ports and commands used by a project",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "path",
						Aliases: []string{"p"},
						Usage:   "Path to the project directory",
						Value:   ".",
					},
				},
				Action: detectProject,
			},
			{
				Name:  "clone-dev",
				Usage: "Clone a repository and run it in development mode",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "repo",
						Aliases:  []string{"r"},
						Usage:    "Git repository URL",
						Required: true,
					},
					&cli.StringFlag{
						Name:    "name",
						Aliases: []string{"n"},
						Usage:   "Project name and directory (optional, defaults to repo name)",
					},
					&cli.StringFlag{
						Name:    "dir",
						Aliases: []string{"d"},
						Usage:   "Custom directory name (optional, overrides name)",
					},
					&cli.BoolFlag{
						Name:    "verbose",
						Aliases: []string{"v"},
						Usage:   "Show verbose output",
					},
				},
				Action: cloneDev,
			},
			{
				Name:  "temporary",
				Usage: "Clone a repository, run it temporarily, and clean up when stopped",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "repo",
						Aliases:  []string{"r"},
						Usage:    "Git repository URL",
						Required: true,
					},
					&cli.StringFlag{
						Name:    "name",
						Aliases: []string{"n"},
						Usage:   "Project name and directory (optional, defaults to repo name)",
					},
					&cli.BoolFlag{
						Name:    "verbose",
						Aliases: []string{"v"},
						Usage:   "Show verbose output",
					},
				},
				Action: temporary,
			},
			{
				Name:   "stop",
				Usage:  "Stop a running project",
				Action: stopProject,
			},
			{
				Name:  "web",
				Usage: "Start the web interface",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "port",
						Aliases: []string{"p"},
						Usage:   "Port to run the web server on",
						Value:   "8080",
					},
				},
				Action: startWebServer,
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatal(err)
	}
}

func cloneProject(c *cli.Context) error {
	repoURL := c.String("repo")
	projectName := c.String("name")
	customDir := c.String("dir")

	return cloneProjectDirect(repoURL, projectName, customDir)
}

func cloneProjectDirect(repoURL, projectName, customDir string) error {
	// Determine the directory name: custom dir > name > auto-generated
	dirName := customDir
	if dirName == "" {
		dirName = projectName
	}
	if dirName == "" {
		// Extract project name from repo URL
		parts := strings.Split(repoURL, "/")
		dirName = strings.TrimSuffix(parts[len(parts)-1], ".git")
	}

	// Create projects directory if it doesn't exist
	projectsDir := "projects"
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		return fmt.Errorf("failed to create projects directory: %v", err)
	}

	projectPath := filepath.Join(projectsDir, dirName)

	// Check if project already exists
	if _, err := os.Stat(projectPath); !os.IsNotExist(err) {
		return fmt.Errorf("project %s already exists", dirName)
	}

	fmt.Printf("Cloning %s into %s...\n", repoURL, projectPath)

	// Clone the repository
	_, err := git.PlainClone(projectPath, false, &git.CloneOptions{
		URL:      repoURL,
		Progress: os.Stdout,
	})
	if err != nil {
		return fmt.Errorf("failed to clone repository: %v", err)
	}

	fmt.Printf("Successfully cloned project %s\n", dirName)
	return nil
}

func runDev(c *cli.Context) error {
	projectName := c.Args().First()
	if projectName == "" {
		return fmt.Errorf("project name is required")
	}

	verbose := c.Bool("verbose")

	projectPath := filepath.Join("projects", projectName)
	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}
	if _, err := os.Stat(absProjectPath); os.IsNotExist(err) {
		return fmt.Errorf("project %s does not exist", projectName)
	}

	projectType := DetectProjectType(absProjectPath)
	fmt.Printf("Detected project type: %s\n", projectType)

	// Get appropriate image and command based on project type
	image, cmd := getDevConfig(projectType, absProjectPath)
	fmt.Printf("Using image: %s, command: %v\n", image, cmd)

	containerName := fmt.Sprintf("sandbox-%s-dev", projectName)

	// Remove existing container if it exists
	exec.Command("docker", "rm", "-f", containerName).Run()

	// Get port mapping for the project type
	hostPort, containerPort := getDevPorts(projectType, absProjectPath)
	fmt.Printf("Using port mapping: %s -> %s\n", hostPort, containerPort)

	// Run docker run command
	dockerArgs := []string{"run"}
	if !verbose {
		dockerArgs = append(dockerArgs, "-d")
	}
	dockerArgs = append(dockerArgs, "--name", containerName)
	if hostPort != "" && containerPort != "" {
		dockerArgs = append(dockerArgs, "-p", fmt.Sprintf("%s:%s", hostPort, containerPort))
	}
	dockerArgs = append(dockerArgs, "-v", fmt.Sprintf("%s:/app", absProjectPath), "-w", "/app", image)
	dockerArgs = append(dockerArgs, cmd...)

	dockerCmd := exec.Command("docker", dockerArgs...)

	if verbose {
		dockerCmd.Stdout = os.Stdout
		dockerCmd.Stderr = os.Stderr
		err = dockerCmd.Run()
		if err != nil {
			return fmt.Errorf("failed to start dev server: %v", err)
		}
		fmt.Printf("✅ Dev server completed for %s\n", projectName)
	} else {
		output, err := dockerCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to start container: %v\nOutput: %s", err, string(output))
		}

		containerID := strings.TrimSpace(string(output))
		fmt.Printf("✅ Started dev container for %s (ID: %s)\n", projectName, containerID[:12])
		if hostPort != "" {
			fmt.Printf("🌐 Dev server available at: http://localhost:%s\n", hostPort)
		}
		fmt.Printf("🐳 Container: %s\n", containerName)
		fmt.Printf("📝 To check logs: docker logs %s\n", containerName)
		fmt.Printf("🛑 To stop: docker stop %s\n", containerName)
	}

	return nil
}

func buildProject(c *cli.Context) error {
	projectName := c.Args().First()
	if projectName == "" {
		return fmt.Errorf("project name is required")
	}

	verbose := c.Bool("verbose")

	projectPath := filepath.Join("projects", projectName)
	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}
	if _, err := os.Stat(absProjectPath); os.IsNotExist(err) {
		return fmt.Errorf("project %s does not exist", projectName)
	}

	projectType := DetectProjectType(absProjectPath)
	fmt.Printf("Detected project type: %s\n", projectType)

	// Get appropriate image and build command based on project type
	image, buildCmd := getBuildConfig(projectType, absProjectPath)

	containerName := fmt.Sprintf("sandbox-%s-build", projectName)

	// Run docker run command
	dockerArgs := []string{"run", "--rm", "--name", containerName,
		"-v", fmt.Sprintf("%s:/app", absProjectPath), "-w", "/app", image}
	dockerArgs = append(dockerArgs, buildCmd...)

	dockerCmd := exec.Command("docker", dockerArgs...)

	fmt.Printf("Building project %s...\n", projectName)

	if verbose {
		dockerCmd.Stdout = os.Stdout
		dockerCmd.Stderr = os.Stderr
		err = dockerCmd.Run()
		if err != nil {
			return fmt.Errorf("failed to build project: %v", err)
		}
	} else {
		output, err := dockerCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to build project: %v\nOutput: %s", err, string(output))
		}
		fmt.Printf("Build output:\n%s\n", string(output))
	}

	fmt.Printf("Successfully built project %s\n", projectName)

	return nil
}

func generateAssets(c *cli.Context) error {
	projectName := c.Args().First()
	if projectName == "" {
		return fmt.Errorf("project name is required")
	}

	projectPath := filepath.Join("projects", projectName)
	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}
	if _, err := os.Stat(absProjectPath); os.IsNotExist(err) {
		return fmt.Errorf("project %s does not exist", projectName)
	}

	projectType := DetectProjectType(absProjectPath)
	fmt.Printf("Detected project type: %s\n", projectType)

	// Get appropriate image and asset generation command based on project type
	image, assetCmd := getAssetConfig(projectType)

	containerName := fmt.Sprintf("sandbox-%s-assets", projectName)

	// Run docker run command
	dockerCmd := exec.Command("docker", "run", "--rm", "--name", containerName,
		"-v", fmt.Sprintf("%s:/app", absProjectPath), "-w", "/app", image)
	dockerCmd.Args = append(dockerCmd.Args, assetCmd...)

	fmt.Printf("Generating assets for project %s...\n", projectName)
	output, err := dockerCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to generate assets: %v\nOutput: %s", err, string(output))
	}

	fmt.Printf("Asset generation output:\n%s\n", string(output))
	fmt.Printf("Successfully generated assets for project %s\n", projectName)

	return nil
}

func serveAssets(c *cli.Context) error {
	projectName := c.Args().First()
	if projectName == "" {
		return fmt.Errorf("project name is required")
	}

	projectPath := filepath.Join("projects", projectName)
	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}
	if _, err := os.Stat(absProjectPath); os.IsNotExist(err) {
		return fmt.Errorf("project %s does not exist", projectName)
	}

	port := c.String("port")
	if port == "" {
		port = "8080"
	}

	// Determine the assets directory based on project type
	projectType := DetectProjectType(absProjectPath)
	assetsDir := getAssetsDirectory(projectType, absProjectPath)

	// Check if assets directory exists
	if _, err := os.Stat(assetsDir); os.IsNotExist(err) {
		return fmt.Errorf("built assets not found in %s. Run 'sandbox build %s' first", assetsDir, projectName)
	}

	containerName := fmt.Sprintf("sandbox-%s-serve", projectName)

	// Remove existing container if it exists
	exec.Command("docker", "rm", "-f", containerName).Run()

	// Run docker run command to serve built assets
	dockerCmd := exec.Command("docker", "run", "-d", "--name", containerName,
		"-v", fmt.Sprintf("%s:/usr/share/nginx/html", assetsDir), "-p", fmt.Sprintf("%s:80", port),
		"nginx:alpine")

	err = dockerCmd.Run()
	if err != nil {
		return fmt.Errorf("failed to start asset server: %v", err)
	}

	fmt.Printf("✅ Started asset server for %s\n", projectName)
	fmt.Printf("🌐 Built assets are being served at: http://localhost:%s\n", port)
	fmt.Printf("📁 Serving from: %s\n", assetsDir)
	fmt.Printf("🐳 Container: sandbox-%s-serve\n", projectName)
	fmt.Printf("📝 To check logs: docker logs sandbox-%s-serve\n", projectName)
	fmt.Printf("🛑 To stop: docker stop sandbox-%s-serve\n", projectName)

	return nil
}

// getDevConfig returns the appropriate Docker image and command for development
func getDevConfig(projectType ProjectType, projectPath string) (string, []string) {
	if handler := GetHandler(projectType); handler != nil {
		return handler.GetDevConfig(projectPath)
	}

	// Fallback for unknown types
	return "ubuntu:22.04", []string{"bash"}
}

// getBuildConfig returns the appropriate Docker image and command for building
func getBuildConfig(projectType ProjectType, projectPath string) (string, []string) {
	if handler := GetHandler(projectType); handler != nil {
		return handler.GetBuildConfig(projectPath)
	}

	// Fallback for unknown types
	return "ubuntu:22.04", []string{"echo", "No build command defined for this project type"}
}

// getAssetConfig returns the appropriate Docker image and command for asset generation
func getAssetConfig(projectType ProjectType) (string, []string) {
	if handler := GetHandler(projectType); handler != nil {
		return handler.GetAssetConfig("")
	}

	// Fallback for unknown types
	return "ubuntu:22.04", []string{"echo", "No asset generation command defined for this project type"}
}

// getDevPorts returns the appropriate port mapping for development servers
func getDevPorts(projectType ProjectType, projectPath string) (string, string) {
	// Use detected ports
	ports := detectPorts(projectType, projectPath)
	if len(ports) > 0 {
		port := ports[0] // Use the first detected port
		return port, port
	}

	// Fallback to defaults
	switch projectType {
	case NodeJS, NodeJSReact, NodeJSNext, NodeJSPnpm:
		return "3000", "3000"
	case NodeJSVite:
		return "5173", "5173" // Vite's default port
	case Go:
		return "8080", "8080"
	case Python:
		return "5000", "5000"
	case PHP:
		return "80", "80"
	case PHPLaravel:
		return "8000", "8000"
	case PHPCodeIgniter:
		return "80", "80"
	case Rust:
		return "8080", "8080"
	case Java:
		return "8080", "8080"
	case CSharp:
		return "5000", "5000"
	case Ruby:
		return "4567", "4567"
	default:
		return "", "" // No port mapping for unknown types
	}
}

// detectNodeJSPort tries to detect the port from package.json scripts
func detectNodeJSPort(projectPath string) string {
	packagePath := filepath.Join(projectPath, "package.json")
	content := readFileContent(packagePath)
	if content == "" {
		return ""
	}

	// Look for common port patterns in scripts
	scripts := extractScriptsFromPackageJSON(content)
	for _, script := range scripts {
		// Check for port flags
		if strings.Contains(script, "--port") || strings.Contains(script, "-p") {
			// Extract port number (simplified)
			if port := extractPortFromScript(script); port != "" {
				return port
			}
		}
		// Check for common dev server ports
		if strings.Contains(script, "vite") {
			return "5173"
		}
		if strings.Contains(script, "next") {
			return "3000"
		}
	}

	return ""
}

// extractScriptsFromPackageJSON extracts scripts from package.json
func extractScriptsFromPackageJSON(content string) map[string]string {
	scripts := make(map[string]string)

	// Simple JSON parsing - look for "scripts" section
	scriptsStart := strings.Index(content, `"scripts"`)
	if scriptsStart == -1 {
		return scripts
	}

	scriptsEnd := strings.Index(content[scriptsStart:], "},")
	if scriptsEnd == -1 {
		scriptsEnd = strings.Index(content[scriptsStart:], "}")
	}
	if scriptsEnd == -1 {
		return scripts
	}

	scriptsSection := content[scriptsStart : scriptsStart+scriptsEnd+1]

	// Extract key-value pairs
	lines := strings.Split(scriptsSection, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, ":") && strings.Contains(line, `"`) {
			parts := strings.Split(line, `"`)
			if len(parts) >= 4 {
				key := strings.Trim(parts[1], `"`)
				value := strings.Trim(parts[3], `",`)
				scripts[key] = value
			}
		}
	}

	return scripts
}

// extractPortFromScript extracts port number from a script command
func extractPortFromScript(script string) string {
	// Look for --port XXXX or -p XXXX patterns
	portPatterns := []string{"--port ", "-p "}
	for _, pattern := range portPatterns {
		if idx := strings.Index(script, pattern); idx != -1 {
			afterPattern := script[idx+len(pattern):]
			// Extract number
			var port string
			for _, char := range afterPattern {
				if char >= '0' && char <= '9' {
					port += string(char)
				} else if port != "" {
					break
				}
			}
			if port != "" {
				return port
			}
		}
	}
	return ""
}

// getAssetsDirectory returns the directory where built assets are located
func getAssetsDirectory(projectType ProjectType, projectPath string) string {
	switch projectType {
	case NodeJS, NodeJSReact, NodeJSNext, NodeJSVite, NodeJSPnpm:
		// Check common build directories
		possibleDirs := []string{"dist", "build", "out", ".next"}
		for _, dir := range possibleDirs {
			fullPath := filepath.Join(projectPath, dir)
			if _, err := os.Stat(fullPath); !os.IsNotExist(err) {
				return fullPath
			}
		}
		// Default to dist for Node.js projects
		return filepath.Join(projectPath, "dist")
	case Go:
		return filepath.Join(projectPath, "dist")
	case Python:
		return filepath.Join(projectPath, "dist")
	case PHP:
		return filepath.Join(projectPath, "public")
	case PHPLaravel:
		return filepath.Join(projectPath, "public")
	case Rust:
		return filepath.Join(projectPath, "target/release")
	case Java:
		return filepath.Join(projectPath, "target")
	case CSharp:
		return filepath.Join(projectPath, "bin/Release")
	case Ruby:
		return filepath.Join(projectPath, "public")
	default:
		return filepath.Join(projectPath, "dist")
	}
}

func listProjects(c *cli.Context) error {
	projectsDir := "projects"
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No projects found.")
			return nil
		}
		return fmt.Errorf("failed to read projects directory: %v", err)
	}

	fmt.Println("Projects:")
	for _, entry := range entries {
		if entry.IsDir() {
			projectPath := filepath.Join(projectsDir, entry.Name())
			projectType := DetectProjectType(projectPath)
			fmt.Printf("  - %s (%s)\n", entry.Name(), projectType)
		}
	}
	return nil
}

func removeProject(c *cli.Context) error {
	projectName := c.Args().First()
	if projectName == "" {
		return fmt.Errorf("project name is required")
	}

	projectPath := filepath.Join("projects", projectName)
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		return fmt.Errorf("project %s does not exist", projectName)
	}

	// Remove associated containers
	containerNames := []string{
		fmt.Sprintf("sandbox-%s-dev", projectName),
		fmt.Sprintf("sandbox-%s-build", projectName),
		fmt.Sprintf("sandbox-%s-assets", projectName),
		fmt.Sprintf("sandbox-%s-serve", projectName),
	}

	for _, containerName := range containerNames {
		exec.Command("docker", "rm", "-f", containerName).Run()
	}

	// Remove project directory
	err := os.RemoveAll(projectPath)
	if err != nil {
		return fmt.Errorf("failed to remove project: %v", err)
	}

	fmt.Printf("Successfully removed project %s\n", projectName)
	return nil
}

func detectProject(c *cli.Context) error {
	projectPath := c.String("path")
	if projectPath == "" {
		projectPath = "."
	}

	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}

	if _, err := os.Stat(absProjectPath); os.IsNotExist(err) {
		return fmt.Errorf("project path %s does not exist", absProjectPath)
	}

	projectType := DetectProjectType(absProjectPath)
	fmt.Printf("Detected project type: %s\n", projectType)

	// Detect ports
	ports := detectPorts(projectType, absProjectPath)
	if len(ports) > 0 {
		fmt.Println("Detected ports:")
		for _, port := range ports {
			fmt.Printf("  - %s\n", port)
		}
	} else {
		fmt.Println("No ports detected")
	}

	// Detect commands
	commands := detectCommands(projectType, absProjectPath)
	if len(commands) > 0 {
		fmt.Println("Detected commands:")
		for script, cmd := range commands {
			fmt.Printf("  - %s: %s\n", script, cmd)
		}
	} else {
		fmt.Println("No commands detected")
	}

	return nil
}

func cloneDev(c *cli.Context) error {
	repoURL := c.String("repo")
	projectName := c.String("name")
	customDir := c.String("dir")

	return cloneDevDirect(repoURL, projectName, customDir)
}

func cloneDevDirect(repoURL, projectName, customDir string) error {
	verbose := false // Default for web API

	// Determine the directory name: custom dir > name > auto-generated
	dirName := customDir
	if dirName == "" {
		dirName = projectName
	}
	if dirName == "" {
		// Extract project name from repo URL
		parts := strings.Split(repoURL, "/")
		dirName = strings.TrimSuffix(parts[len(parts)-1], ".git")
	}

	// Create projects directory if it doesn't exist
	projectsDir := "projects"
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		return fmt.Errorf("failed to create projects directory: %v", err)
	}

	projectPath := filepath.Join(projectsDir, dirName)

	// Check if project already exists
	if _, err := os.Stat(projectPath); !os.IsNotExist(err) {
		return fmt.Errorf("project %s already exists", dirName)
	}

	fmt.Printf("Cloning %s into %s...\n", repoURL, projectPath)

	// Clone the repository
	_, err := git.PlainClone(projectPath, false, &git.CloneOptions{
		URL:      repoURL,
		Progress: os.Stdout,
	})
	if err != nil {
		return fmt.Errorf("failed to clone repository: %v", err)
	}

	fmt.Printf("Successfully cloned project %s\n", dirName)

	// Now run dev on the cloned project
	return runDevOnProject(dirName, verbose)
}

func temporary(c *cli.Context) error {
	repoURL := c.String("repo")
	projectName := c.String("name")
	verbose := c.Bool("verbose")

	// Use a temporary name if not provided
	if projectName == "" {
		parts := strings.Split(repoURL, "/")
		projectName = "temp-" + strings.TrimSuffix(parts[len(parts)-1], ".git") + "-" + fmt.Sprintf("%d", time.Now().Unix())
	}

	// Clone the project
	err := cloneProjectInternal(repoURL, projectName)
	if err != nil {
		return err
	}

	// Run dev
	fmt.Printf("Running %s temporarily (press Ctrl+C to stop and clean up)...\n", projectName)

	// Set up signal handling for cleanup
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, os.Kill)

	// Run dev in background
	go func() {
		runDevOnProject(projectName, verbose)
	}()

	// Wait for signal
	<-signalChan

	fmt.Printf("\nStopping and cleaning up %s...\n", projectName)

	// Remove containers and project directory
	err = removeProject(c)
	if err != nil {
		fmt.Printf("Warning: failed to remove project: %v\n", err)
		return err
	}
	fmt.Printf("Temporary project %s cleaned up\n", projectName)
	return nil
}

func stopProject(c *cli.Context) error {
	projectName := c.Args().First()
	if projectName == "" {
		return fmt.Errorf("project name is required")
	}

	fmt.Printf("Stopping project %s...\n", projectName)
	stopProjectContainers(projectName)
	fmt.Printf("Successfully stopped project %s\n", projectName)
	return nil
}

func runDevOnProject(projectName string, verbose bool) error {
	projectPath := filepath.Join("projects", projectName)
	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}
	if _, err := os.Stat(absProjectPath); os.IsNotExist(err) {
		return fmt.Errorf("project %s does not exist", projectName)
	}

	projectType := DetectProjectType(absProjectPath)
	fmt.Printf("Detected project type: %s\n", projectType)

	// Get appropriate image and command based on project type
	image, cmd := getDevConfig(projectType, absProjectPath)

	containerName := fmt.Sprintf("sandbox-%s-dev", projectName)

	// Remove existing container if it exists
	exec.Command("docker", "rm", "-f", containerName).Run()

	// Get port mapping for the project type
	hostPort, containerPort := getDevPorts(projectType, absProjectPath)

	// Run docker run command
	dockerArgs := []string{"run"}
	if !verbose {
		dockerArgs = append(dockerArgs, "-d")
	}
	dockerArgs = append(dockerArgs, "--name", containerName)
	if hostPort != "" && containerPort != "" {
		dockerArgs = append(dockerArgs, "-p", fmt.Sprintf("%s:%s", hostPort, containerPort))
	}
	dockerArgs = append(dockerArgs, "-v", fmt.Sprintf("%s:/app", absProjectPath), "-w", "/app", image)
	dockerArgs = append(dockerArgs, cmd...)

	dockerCmd := exec.Command("docker", dockerArgs...)

	if verbose {
		dockerCmd.Stdout = os.Stdout
		dockerCmd.Stderr = os.Stderr
		err = dockerCmd.Run()
		if err != nil {
			return fmt.Errorf("failed to start dev server: %v", err)
		}
		fmt.Printf("✅ Dev server completed for %s\n", projectName)
	} else {
		output, err := dockerCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to start container: %v\nOutput: %s", err, string(output))
		}

		containerID := strings.TrimSpace(string(output))
		fmt.Printf("✅ Started dev container for %s (ID: %s)\n", projectName, containerID[:12])
		if hostPort != "" {
			fmt.Printf("🌐 Dev server available at: http://localhost:%s\n", hostPort)
		}
		fmt.Printf("🐳 Container: %s\n", containerName)
		fmt.Printf("📝 To check logs: docker logs %s\n", containerName)
		fmt.Printf("🛑 To stop: docker stop %s\n", containerName)
	}

	return nil
}

func cloneProjectInternal(repoURL, projectName string) error {
	// Create projects directory if it doesn't exist
	projectsDir := "projects"
	if err := os.MkdirAll(projectsDir, 0755); err != nil {
		return fmt.Errorf("failed to create projects directory: %v", err)
	}

	projectPath := filepath.Join(projectsDir, projectName)

	// Check if project already exists
	if _, err := os.Stat(projectPath); !os.IsNotExist(err) {
		return fmt.Errorf("project %s already exists", projectName)
	}

	fmt.Printf("Cloning %s into %s...\n", repoURL, projectPath)

	// Clone the repository
	_, err := git.PlainClone(projectPath, false, &git.CloneOptions{
		URL:      repoURL,
		Progress: os.Stdout,
	})
	if err != nil {
		return fmt.Errorf("failed to clone repository: %v", err)
	}

	fmt.Printf("Successfully cloned project %s\n", projectName)
	return nil
}

func stopProjectContainers(projectName string) {
	containerNames := []string{
		fmt.Sprintf("sandbox-%s-dev", projectName),
		fmt.Sprintf("sandbox-%s-build", projectName),
		fmt.Sprintf("sandbox-%s-assets", projectName),
		fmt.Sprintf("sandbox-%s-serve", projectName),
	}

	for _, containerName := range containerNames {
		exec.Command("docker", "stop", containerName).Run()
	}
}

// isProjectRunning checks if any containers for the project are running
func isProjectRunning(projectName string) bool {
	containerNames := []string{
		fmt.Sprintf("sandbox-%s-dev", projectName),
		fmt.Sprintf("sandbox-%s-build", projectName),
		fmt.Sprintf("sandbox-%s-assets", projectName),
		fmt.Sprintf("sandbox-%s-serve", projectName),
	}

	for _, containerName := range containerNames {
		cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", containerName), "--filter", "status=running", "--format", "{{.Names}}")
		output, err := cmd.Output()
		if err == nil && strings.TrimSpace(string(output)) != "" {
			return true
		}
	}

	return false
}

// getProjectRunningPort gets the port that a running project is using
func getProjectRunningPort(projectName string) string {
	containerNames := []string{
		fmt.Sprintf("sandbox-%s-dev", projectName),
		fmt.Sprintf("sandbox-%s-build", projectName),
		fmt.Sprintf("sandbox-%s-assets", projectName),
		fmt.Sprintf("sandbox-%s-serve", projectName),
	}

	for _, containerName := range containerNames {
		// Get port mappings for running containers
		cmd := exec.Command("docker", "ps", "--filter", fmt.Sprintf("name=%s", containerName), "--filter", "status=running", "--format", "{{.Ports}}")
		output, err := cmd.Output()
		if err == nil {
			portsStr := strings.TrimSpace(string(output))
			if portsStr != "" {
				// Parse port mapping like "0.0.0.0:3000->3000/tcp"
				parts := strings.Split(portsStr, "->")
				if len(parts) > 0 {
					hostPort := strings.Split(parts[0], ":")
					if len(hostPort) > 1 {
						return hostPort[1]
					}
				}
			}
		}
	}

	return ""
}

// getProjectReadme reads the README.md file
func getProjectReadme(projectPath string) string {
	readmePath := filepath.Join(projectPath, "README.md")
	content := readFileContent(readmePath)
	if content == "" {
		return ""
	}
	return content
}

// getProjectDescription extracts description from project files
func getProjectDescription(projectPath string, projectType ProjectType) string {
	switch projectType {
	case NodeJS, NodeJSReact, NodeJSNext, NodeJSVite, NodeJSPnpm:
		packagePath := filepath.Join(projectPath, "package.json")
		content := readFileContent(packagePath)
		if content != "" {
			// Simple extraction of description field
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
	case Go:
		// Could extract from go.mod comments or README
		return "Go application"
	default:
		return ""
	}
	return ""
}

// getProjectDependencies extracts dependencies from project files
func getProjectDependencies(projectPath string, projectType ProjectType) []string {
	var deps []string

	switch projectType {
	case NodeJS, NodeJSReact, NodeJSNext, NodeJSVite, NodeJSPnpm:
		packagePath := filepath.Join(projectPath, "package.json")
		content := readFileContent(packagePath)
		if content != "" {
			// Extract dependencies
			depsStart := strings.Index(content, `"dependencies"`)
			if depsStart != -1 {
				depsEnd := strings.Index(content[depsStart:], "},")
				if depsEnd == -1 {
					depsEnd = strings.Index(content[depsStart:], "}")
				}
				if depsEnd != -1 {
					depsSection := content[depsStart : depsStart+depsEnd+1]
					// Simple extraction of package names
					lines := strings.Split(depsSection, "\n")
					for _, line := range lines {
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
		}
	case Go:
		info, err := ParseGoMod(filepath.Join(projectPath, "go.mod"))
		if err != nil {
			return nil
		}
		for _, dep := range info.Direct {
			deps = append(deps, dep.Path)
		}
	}

	// Limit to first 10 dependencies
	if len(deps) > 10 {
		deps = deps[:10]
	}

	return deps
}

type Dependency struct {
	Path    string
	Version string
}

type Replace struct {
	OldPath    string
	OldVersion string
	NewPath    string
	NewVersion string
}

type GoModInfo struct {
	Direct    []Dependency
	Indirect  []Dependency
	Replaced  []Replace
	Module    string
	GoVersion string
}

// ParseGoMod parses go.mod at the given path and extracts dependencies info
func ParseGoMod(path string) (*GoModInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	f, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return nil, err
	}

	info := &GoModInfo{
		Direct:    []Dependency{},
		Indirect:  []Dependency{},
		Replaced:  []Replace{},
		Module:    f.Module.Mod.Path,
		GoVersion: f.Go.Version,
	}

	for _, r := range f.Require {
		dep := Dependency{
			Path:    r.Mod.Path,
			Version: r.Mod.Version,
		}
		if r.Indirect {
			info.Indirect = append(info.Indirect, dep)
		} else {
			info.Direct = append(info.Direct, dep)
		}
	}

	for _, r := range f.Replace {
		rep := Replace{
			OldPath:    r.Old.Path,
			OldVersion: r.Old.Version,
			NewPath:    r.New.Path,
			NewVersion: r.New.Version,
		}
		info.Replaced = append(info.Replaced, rep)
	}

	return info, nil
}

// getProjectScripts extracts scripts from project files
func getProjectScripts(projectPath string, projectType ProjectType) map[string]string {
	switch projectType {
	case NodeJS, NodeJSReact, NodeJSNext, NodeJSVite, NodeJSPnpm:
		packagePath := filepath.Join(projectPath, "package.json")
		content := readFileContent(packagePath)
		if content != "" {
			return extractScriptsFromPackageJSON(content)
		}
	}
	return nil
}

func buildProjectOnProject(projectName string, verbose bool) error {
	projectPath := filepath.Join("projects", projectName)
	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %v", err)
	}
	if _, err := os.Stat(absProjectPath); os.IsNotExist(err) {
		return fmt.Errorf("project %s does not exist", projectName)
	}

	projectType := DetectProjectType(absProjectPath)
	fmt.Printf("Detected project type: %s\n", projectType)

	// Get appropriate image and build command based on project type
	image, buildCmd := getBuildConfig(projectType, absProjectPath)

	containerName := fmt.Sprintf("sandbox-%s-build", projectName)

	// Run docker run command
	dockerArgs := []string{"run", "--rm", "--name", containerName,
		"-v", fmt.Sprintf("%s:/app", absProjectPath), "-w", "/app", image}
	dockerArgs = append(dockerArgs, buildCmd...)

	dockerCmd := exec.Command("docker", dockerArgs...)

	fmt.Printf("Building project %s...\n", projectName)

	if verbose {
		dockerCmd.Stdout = os.Stdout
		dockerCmd.Stderr = os.Stderr
		err = dockerCmd.Run()
		if err != nil {
			return fmt.Errorf("failed to build project: %v", err)
		}
	} else {
		output, err := dockerCmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("failed to build project: %v\nOutput: %s", err, string(output))
		}
		fmt.Printf("Build output:\n%s\n", string(output))
	}

	fmt.Printf("Successfully built project %s\n", projectName)

	return nil
}

// detectPorts detects ports used by the project
func detectPorts(projectType ProjectType, projectPath string) []string {
	var ports []string

	switch projectType {
	case NodeJS, NodeJSReact, NodeJSNext, NodeJSVite, NodeJSPnpm:
		if port := detectNodeJSPort(projectPath); port != "" {
			ports = append(ports, port)
		}
	case Go:
		goPorts := detectGoPorts(projectPath)
		ports = append(ports, goPorts...)
	default:
		// Default ports
		if defaultPort := getDefaultPort(projectType); defaultPort != "" {
			ports = append(ports, defaultPort)
		}
	}

	return ports
}

// detectCommands detects commands/scripts used by the project
func detectCommands(projectType ProjectType, projectPath string) map[string]string {
	commands := make(map[string]string)

	switch projectType {
	case NodeJS, NodeJSReact, NodeJSNext, NodeJSVite, NodeJSPnpm:
		packagePath := filepath.Join(projectPath, "package.json")
		content := readFileContent(packagePath)
		if content != "" {
			scripts := extractScriptsFromPackageJSON(content)
			for script, cmd := range scripts {
				commands[script] = cmd
			}
		}
	case Go:
		// Get Go version
		if version := getGoVersion(projectPath); version != "" {
			commands["go-version"] = version
		}

		// Basic Go commands
		commands["run"] = "go run ."
		commands["build"] = "go build ."
		commands["test"] = "go test ./..."

		// Check for CLI frameworks
		if detectCLICobra(projectPath) {
			commands["cli"] = "Uses Cobra CLI framework"
		}
		if detectCLIUrfave(projectPath) {
			commands["cli"] = "Uses Urfave CLI framework"
		}
	default:
		// Add default commands if any
	}

	return commands
}

// detectGoPorts detects ports from Go source files
func detectGoPorts(projectPath string) []string {
	var ports []string

	// Walk through .go files
	filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		content := readFileContent(path)
		// Look for http.ListenAndServe patterns
		lines := strings.Split(content, "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.Contains(line, "ListenAndServe") || strings.Contains(line, "Listen") {
				// Extract port from :port or "port"
				if port := extractPortFromGoLine(line); port != "" {
					ports = append(ports, port)
				}
			}
		}

		return nil
	})

	return ports
}

// extractPortFromGoLine extracts port from a Go line like app.Listen(":8080")
func extractPortFromGoLine(line string) string {
	// Simple extraction
	start := strings.Index(line, `"`)
	if start == -1 {
		start = strings.Index(line, ":")
		if start == -1 {
			return ""
		}
	} else {
		start++
	}

	end := strings.Index(line[start:], `"`)
	if end == -1 {
		end = strings.Index(line[start:], ")")
		if end == -1 {
			return ""
		}
	}

	portStr := line[start : start+end]
	// Remove :
	portStr = strings.TrimPrefix(portStr, ":")

	// Check if it's a number
	if _, err := strconv.Atoi(portStr); err == nil {
		return portStr
	}

	return ""
}

// getDefaultPort returns default port for project type
func getDefaultPort(projectType ProjectType) string {
	switch projectType {
	case NodeJS, NodeJSReact, NodeJSNext, NodeJSPnpm:
		return "3000"
	case NodeJSVite:
		return "5173"
	case Go:
		return "8080"
	case Python:
		return "5000"
	case PHP, PHPCodeIgniter:
		return "80"
	case PHPLaravel:
		return "8000"
	case Rust:
		return "8080"
	case Java:
		return "8080"
	case CSharp:
		return "5000"
	case Ruby:
		return "4567"
	default:
		return ""
	}
}

// detectCLICobra checks if the Go project uses Cobra CLI
func detectCLICobra(projectPath string) bool {
	return checkImportInGoFiles(projectPath, "github.com/spf13/cobra")
}

// detectCLIUrfave checks if the Go project uses Urfave CLI
func detectCLIUrfave(projectPath string) bool {
	return checkImportInGoFiles(projectPath, "github.com/urfave/cli")
}

// checkImportInGoFiles checks if a specific import exists in any .go file
func checkImportInGoFiles(projectPath, importPath string) bool {
	found := false
	filepath.Walk(projectPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}

		content := readFileContent(path)
		if strings.Contains(content, importPath) {
			found = true
			return filepath.SkipAll // Stop walking
		}

		return nil
	})
	return found
}

// getGoVersion extracts Go version from go.mod
func getGoVersion(projectPath string) string {
	goModPath := filepath.Join(projectPath, "go.mod")
	content := readFileContent(goModPath)
	if content == "" {
		return ""
	}

	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "go ") {
			return strings.TrimPrefix(line, "go ")
		}
	}
	return ""
}

// API Response structures
type APIResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

type ProjectInfo struct {
	Name         string            `json:"name"`
	Type         ProjectType       `json:"type"`
	Path         string            `json:"path"`
	Running      bool              `json:"running"`
	Port         string            `json:"port,omitempty"`
	Readme       string            `json:"readme,omitempty"`
	Description  string            `json:"description,omitempty"`
	Dependencies []string          `json:"dependencies,omitempty"`
	Scripts      map[string]string `json:"scripts,omitempty"`
	GoVersion    string            `json:"go_version,omitempty"`
}

type CloneRequest struct {
	Repo string `json:"repo"`
	Name string `json:"name,omitempty"`
	Dir  string `json:"dir,omitempty"`
}

type OperationRequest struct {
	Project string `json:"project"`
	Verbose bool   `json:"verbose,omitempty"`
}

// startWebServer starts the web interface
func startWebServer(c *cli.Context) error {
	port := c.String("port")

	// Setup routes
	http.HandleFunc("/", serveFrontend)
	http.HandleFunc("/api/projects", handleProjects)
	http.HandleFunc("/api/projects/clone", handleClone)
	http.HandleFunc("/api/projects/clone-dev", handleCloneDev)
	http.HandleFunc("/api/projects/temporary", handleTemporary)
	http.HandleFunc("/api/projects/dev", handleDev)
	http.HandleFunc("/api/projects/build", handleBuild)
	http.HandleFunc("/api/projects/stop", handleStop)
	http.HandleFunc("/api/projects/remove", handleRemove)
	http.HandleFunc("/api/projects/detect", handleDetect)

	fmt.Printf("🚀 Starting web interface on http://localhost:%s\n", port)
	fmt.Println("📱 Open your browser to access the web interface")
	fmt.Println("🛑 Press Ctrl+C to stop the server")

	return http.ListenAndServe(":"+port, nil)
}

// serveFrontend serves the HTML frontend
func serveFrontend(w http.ResponseWriter, r *http.Request) {
	// Enable CORS
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		return
	}

	html, err := os.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, "Frontend not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write(html)
}

// API Handlers
func handleProjects(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != "GET" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Method not allowed"})
		return
	}

	projectsDir := "projects"
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			json.NewEncoder(w).Encode(APIResponse{Success: true, Data: []ProjectInfo{}})
			return
		}
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: err.Error()})
		return
	}

	var projects []ProjectInfo
	for _, entry := range entries {
		if entry.IsDir() {
			projectPath := filepath.Join(projectsDir, entry.Name())
			projectType := DetectProjectType(projectPath)
			absPath, _ := filepath.Abs(projectPath)
			running := isProjectRunning(entry.Name())
			port := ""
			if running {
				port = getProjectRunningPort(entry.Name())
			}

			// Get additional project details
			readme := getProjectReadme(projectPath)
			description := getProjectDescription(projectPath, projectType)
			dependencies := getProjectDependencies(projectPath, projectType)
			scripts := getProjectScripts(projectPath, projectType)
			goVersion := getGoVersion(projectPath)

			projects = append(projects, ProjectInfo{
				Name:         entry.Name(),
				Type:         projectType,
				Path:         absPath,
				Running:      running,
				Port:         port,
				Readme:       readme,
				Description:  description,
				Dependencies: dependencies,
				Scripts:      scripts,
				GoVersion:    goVersion,
			})
		}
	}

	json.NewEncoder(w).Encode(APIResponse{Success: true, Data: projects})
}

func handleClone(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Method not allowed"})
		return
	}

	var req CloneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Invalid JSON"})
		return
	}

	if req.Repo == "" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Repository URL is required"})
		return
	}

	err := cloneProjectDirect(req.Repo, req.Name, req.Dir)
	if err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(APIResponse{Success: true, Message: "Project cloned successfully"})
}

func handleCloneDev(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Method not allowed"})
		return
	}

	var req CloneRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Invalid JSON"})
		return
	}

	if req.Repo == "" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Repository URL is required"})
		return
	}

	err := cloneDevDirect(req.Repo, req.Name, req.Dir)
	if err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Failed to clone and start project: " + err.Error()})
		return
	}

	json.NewEncoder(w).Encode(APIResponse{Success: true, Message: "Project cloned and started successfully"})
}

func handleTemporary(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Method not allowed"})
		return
	}

	var req struct {
		Repo string `json:"repo"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Invalid JSON"})
		return
	}

	if req.Repo == "" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Repository URL is required"})
		return
	}

	// Simulate CLI context
	c := &cli.Context{}
	c.Set("repo", req.Repo)

	err := temporary(c)
	if err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Failed to run temporary project: " + err.Error()})
		return
	}

	json.NewEncoder(w).Encode(APIResponse{Success: true, Message: "Temporary project completed and cleaned up"})
}

func handleDev(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Method not allowed"})
		return
	}

	var req OperationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Invalid JSON"})
		return
	}

	if req.Project == "" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Project name is required"})
		return
	}

	err := runDevOnProject(req.Project, req.Verbose)
	if err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(APIResponse{Success: true, Message: "Development server started"})
}

func handleBuild(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Method not allowed"})
		return
	}

	var req OperationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Invalid JSON"})
		return
	}

	if req.Project == "" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Project name is required"})
		return
	}

	err := buildProjectOnProject(req.Project, req.Verbose)
	if err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(APIResponse{Success: true, Message: "Project built successfully"})
}

func handleStop(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Method not allowed"})
		return
	}

	var req OperationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Invalid JSON"})
		return
	}

	if req.Project == "" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Project name is required"})
		return
	}

	fmt.Printf("Stopping project %s...\n", req.Project)
	stopProjectContainers(req.Project)
	fmt.Printf("Successfully stopped project %s\n", req.Project)

	json.NewEncoder(w).Encode(APIResponse{Success: true, Message: "Project stopped successfully"})
}

func handleRemove(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Method not allowed"})
		return
	}

	var req OperationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Invalid JSON"})
		return
	}

	if req.Project == "" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Project name is required"})
		return
	}

	projectPath := filepath.Join("projects", req.Project)
	if _, err := os.Stat(projectPath); os.IsNotExist(err) {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Project does not exist"})
		return
	}

	// Stop associated containers
	stopProjectContainers(req.Project)

	// Remove project directory
	err := os.RemoveAll(projectPath)
	if err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: err.Error()})
		return
	}

	fmt.Printf("Successfully removed project %s\n", req.Project)
	json.NewEncoder(w).Encode(APIResponse{Success: true, Message: "Project removed successfully"})
}

func handleDetect(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Method not allowed"})
		return
	}

	var req OperationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Invalid JSON"})
		return
	}

	if req.Project == "" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Project name is required"})
		return
	}

	projectPath := filepath.Join("projects", req.Project)
	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: err.Error()})
		return
	}

	projectType := DetectProjectType(absProjectPath)
	ports := detectPorts(projectType, absProjectPath)
	commands := detectCommands(projectType, absProjectPath)

	data := map[string]interface{}{
		"type":     projectType,
		"ports":    ports,
		"commands": commands,
	}

	json.NewEncoder(w).Encode(APIResponse{Success: true, Data: data})
}

// DetectProjectType detects the project type based on files in the directory
func DetectProjectType(projectPath string) ProjectType {
	// Check handlers first (more extensible)
	for projectType, handler := range handlers {
		if handler.Detect(projectPath) {
			return projectType
		}
	}

	// Fallback to unknown if no handler matches
	return Unknown
}

// fileExists checks if a file exists
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return !os.IsNotExist(err)
}

// readFileContent reads the content of a file as string
func readFileContent(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(content)
}
