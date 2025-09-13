package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/urfave/cli/v2"
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

func main() {
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
	image, buildCmd := getBuildConfig(projectType)

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
	switch projectType {
	case NodeJS, NodeJSReact, NodeJSNext, NodeJSVite, NodeJSPnpm:
		// Use npm/pnpm run dev with host flag
		if projectType == NodeJSPnpm || projectType == NodeJSVite {
			return "node:22", []string{"sh", "-c", "npm install -g pnpm && pnpm install && pnpm run dev --host 0.0.0.0"}
		}
		return "node:22", []string{"sh", "-c", "npm install && npm run dev -- --host 0.0.0.0"}
	case Go:
		// Detect Go version from go.mod
		goVersion := getGoVersion(projectPath)
		if goVersion != "" {
			// Extract major.minor (e.g., "1.25" from "1.25.0")
			parts := strings.Split(goVersion, ".")
			if len(parts) >= 2 {
				imageVersion := parts[0] + "." + parts[1]
				return "golang:" + imageVersion, []string{"go", "run", "."}
			}
		}
		return "golang:1.22", []string{"go", "run", "."}
	case Python:
		return "python:3.11", []string{"sh", "-c", "pip install -r requirements.txt && python app.py"}
	case PHP:
		return "php:8.2-apache", []string{"apache2-foreground"}
	case PHPLaravel:
		return "php:8.2-apache", []string{"sh", "-c", "composer install && php artisan serve --host=0.0.0.0 --port=8000"}
	case PHPCodeIgniter:
		return "php:8.2-apache", []string{"sh", "-c", "composer install && apache2-foreground"}
	case Rust:
		return "rust:1.70", []string{"cargo", "run"}
	case Java:
		return "openjdk:17", []string{"sh", "-c", "javac *.java && java Main"}
	case CSharp:
		return "mcr.microsoft.com/dotnet/sdk:7.0", []string{"sh", "-c", "dotnet restore && dotnet run"}
	case Ruby:
		return "ruby:3.2", []string{"sh", "-c", "bundle install && ruby app.rb"}
	default:
		return "ubuntu:22.04", []string{"bash"}
	}
}

// getBuildConfig returns the appropriate Docker image and command for building
func getBuildConfig(projectType ProjectType) (string, []string) {
	switch projectType {
	case NodeJS:
		return "node:22", []string{"sh", "-c", "npm install && npm run build"}
	case NodeJSReact:
		return "node:22", []string{"sh", "-c", "npm install && npm run build"}
	case NodeJSNext:
		return "node:22", []string{"sh", "-c", "npm install && npm run build"}
	case NodeJSVite:
		return "node:22", []string{"sh", "-c", "npm install -g pnpm && pnpm install && pnpm run build"}
	case NodeJSPnpm:
		return "node:22", []string{"sh", "-c", "npm install -g pnpm && pnpm install && pnpm run build"}
	case Go:
		return "golang:1.21", []string{"go", "build", "."}
	case Python:
		return "python:3.11", []string{"python", "setup.py", "build"}
	case PHP:
		return "php:8.2-cli", []string{"composer", "install", "--no-dev", "--optimize-autoloader"}
	case PHPLaravel:
		return "php:8.2-cli", []string{"sh", "-c", "composer install --no-dev --optimize-autoloader && php artisan config:cache && php artisan route:cache && php artisan view:cache"}
	case PHPCodeIgniter:
		return "php:8.2-cli", []string{"composer", "install", "--no-dev", "--optimize-autoloader"}
	case Rust:
		return "rust:1.70", []string{"cargo", "build", "--release"}
	case Java:
		return "openjdk:17", []string{"javac", "*.java"}
	case CSharp:
		return "mcr.microsoft.com/dotnet/sdk:7.0", []string{"dotnet", "publish", "-c", "Release"}
	case Ruby:
		return "ruby:3.2", []string{"bundle", "exec", "rake", "build"}
	default:
		return "ubuntu:22.04", []string{"echo", "No build command defined for this project type"}
	}
}

// getAssetConfig returns the appropriate Docker image and command for asset generation
func getAssetConfig(projectType ProjectType) (string, []string) {
	switch projectType {
	case NodeJS:
		return "node:22", []string{"sh", "-c", "npm install && npm run generate"}
	case NodeJSReact:
		return "node:22", []string{"sh", "-c", "npm install && npm run build"}
	case NodeJSNext:
		return "node:22", []string{"sh", "-c", "npm install && npm run build"}
	case NodeJSVite:
		return "node:22", []string{"sh", "-c", "npm install -g pnpm && pnpm install && pnpm run build"}
	case NodeJSPnpm:
		return "node:22", []string{"sh", "-c", "npm install -g pnpm && pnpm install && pnpm run generate"}
	case Go:
		return "golang:1.21", []string{"echo", "Asset generation not typically needed for Go projects"}
	case Python:
		return "python:3.11", []string{"echo", "Asset generation not defined for Python projects"}
	case PHP:
		return "php:8.2-cli", []string{"echo", "Asset generation not defined for PHP projects"}
	case PHPLaravel:
		return "php:8.2-cli", []string{"sh", "-c", "composer install && php artisan vite:build"}
	case PHPCodeIgniter:
		return "php:8.2-cli", []string{"echo", "Asset generation not defined for CodeIgniter projects"}
	case Rust:
		return "rust:1.70", []string{"echo", "Asset generation not defined for Rust projects"}
	case Java:
		return "openjdk:17", []string{"echo", "Asset generation not defined for Java projects"}
	case CSharp:
		return "mcr.microsoft.com/dotnet/sdk:7.0", []string{"echo", "Asset generation not defined for C# projects"}
	case Ruby:
		return "ruby:3.2", []string{"echo", "Asset generation not defined for Ruby projects"}
	default:
		return "ubuntu:22.04", []string{"echo", "No asset generation command defined for this project type"}
	}
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
	verbose := c.Bool("verbose")

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

// DetectProjectType detects the project type based on files in the directory
func DetectProjectType(projectPath string) ProjectType {
	// Check for PHP frameworks first (more specific)
	if fileExists(filepath.Join(projectPath, "composer.json")) {
		composerContent := readFileContent(filepath.Join(projectPath, "composer.json"))
		if strings.Contains(composerContent, "laravel/framework") {
			return PHPLaravel
		}
		return PHP
	}

	// Check for CodeIgniter
	if fileExists(filepath.Join(projectPath, "system")) &&
		fileExists(filepath.Join(projectPath, "application")) {
		return PHPCodeIgniter
	}

	// Check for Node.js variants
	if fileExists(filepath.Join(projectPath, "package.json")) {
		packageContent := readFileContent(filepath.Join(projectPath, "package.json"))

		// Check for Next.js
		if strings.Contains(packageContent, "next") ||
			fileExists(filepath.Join(projectPath, "next.config.js")) ||
			fileExists(filepath.Join(projectPath, "next.config.mjs")) {
			return NodeJSNext
		}

		// Check for React
		if strings.Contains(packageContent, "react") &&
			!strings.Contains(packageContent, "next") {
			return NodeJSReact
		}

		// Check for Vite
		if strings.Contains(packageContent, "vite") ||
			fileExists(filepath.Join(projectPath, "vite.config.js")) ||
			fileExists(filepath.Join(projectPath, "vite.config.ts")) {
			return NodeJSVite
		}

		// Check for pnpm
		if fileExists(filepath.Join(projectPath, "pnpm-lock.yaml")) ||
			fileExists(filepath.Join(projectPath, "pnpm-workspace.yaml")) {
			return NodeJSPnpm
		}

		return NodeJS
	}

	// Check for Go
	if fileExists(filepath.Join(projectPath, "go.mod")) {
		return Go
	}

	// Check for Python
	if fileExists(filepath.Join(projectPath, "requirements.txt")) ||
		fileExists(filepath.Join(projectPath, "setup.py")) ||
		fileExists(filepath.Join(projectPath, "pyproject.toml")) {
		return Python
	}

	// Check for Rust
	if fileExists(filepath.Join(projectPath, "Cargo.toml")) {
		return Rust
	}

	// Check for Java
	if fileExists(filepath.Join(projectPath, "pom.xml")) ||
		fileExists(filepath.Join(projectPath, "build.gradle")) ||
		fileExists(filepath.Join(projectPath, "build.gradle.kts")) {
		return Java
	}

	// Check for C#
	if fileExists(filepath.Join(projectPath, "*.csproj")) ||
		fileExists(filepath.Join(projectPath, "Program.cs")) {
		return CSharp
	}

	// Check for Ruby
	if fileExists(filepath.Join(projectPath, "Gemfile")) ||
		fileExists(filepath.Join(projectPath, "Rakefile")) {
		return Ruby
	}

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
