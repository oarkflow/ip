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
	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"
	"github.com/urfave/cli/v2"
	"golang.org/x/mod/modfile"
)

// Group represents a collection of related projects
type Group struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Projects    []GroupProject `json:"projects"`
	Path        string         `json:"path"`
	Created     time.Time      `json:"created"`
}

// GroupProject represents a project within a group with dependencies
type GroupProject struct {
	Name         string            `json:"name"`
	Path         string            `json:"path"`
	Dependencies []string          `json:"dependencies"`         // Names of other projects in the group
	AssetDirs    map[string]string `json:"asset_dirs,omitempty"` // Custom asset directories for dependencies
}

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

// Group management functions
func createGroup(name, description string) error {
	groupsDir := "groups"
	if err := os.MkdirAll(groupsDir, 0755); err != nil {
		return fmt.Errorf("failed to create groups directory: %v", err)
	}

	groupPath := filepath.Join(groupsDir, name+".json")
	if _, err := os.Stat(groupPath); !os.IsNotExist(err) {
		return fmt.Errorf("group %s already exists", name)
	}

	group := Group{
		Name:        name,
		Description: description,
		Projects:    []GroupProject{},
		Path:        groupPath,
		Created:     time.Now(),
	}

	data, err := json.MarshalIndent(group, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal group: %v", err)
	}

	if err := os.WriteFile(groupPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write group file: %v", err)
	}

	fmt.Printf("✅ Created group '%s'\n", name)
	return nil
}

func loadGroup(name string) (*Group, error) {
	groupPath := filepath.Join("groups", name+".json")
	data, err := os.ReadFile(groupPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read group file: %v", err)
	}

	var group Group
	if err := json.Unmarshal(data, &group); err != nil {
		return nil, fmt.Errorf("failed to unmarshal group: %v", err)
	}

	return &group, nil
}

func saveGroup(group *Group) error {
	data, err := json.MarshalIndent(group, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal group: %v", err)
	}

	if err := os.WriteFile(group.Path, data, 0644); err != nil {
		return fmt.Errorf("failed to write group file: %v", err)
	}

	return nil
}

func addProjectToGroup(groupName, projectName string, dependencies []string) error {
	group, err := loadGroup(groupName)
	if err != nil {
		return err
	}

	// Check if project already exists in group
	for _, p := range group.Projects {
		if p.Name == projectName {
			return fmt.Errorf("project %s already exists in group %s", projectName, groupName)
		}
	}

	// Validate dependencies exist in group
	for _, dep := range dependencies {
		found := false
		for _, p := range group.Projects {
			if p.Name == dep {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("dependency %s not found in group %s", dep, groupName)
		}
	}

	projectPath := filepath.Join("projects", projectName)
	groupProject := GroupProject{
		Name:         projectName,
		Path:         projectPath,
		Dependencies: dependencies,
	}

	group.Projects = append(group.Projects, groupProject)

	if err := saveGroup(group); err != nil {
		return err
	}

	fmt.Printf("✅ Added project '%s' to group '%s'\n", projectName, groupName)
	return nil
}

func resolveBuildOrder(group *Group) ([]GroupProject, error) {
	// Simple topological sort for dependency resolution
	var result []GroupProject
	visited := make(map[string]bool)
	visiting := make(map[string]bool)

	var visit func(project GroupProject) error
	visit = func(project GroupProject) error {
		if visiting[project.Name] {
			return fmt.Errorf("circular dependency detected involving %s", project.Name)
		}
		if visited[project.Name] {
			return nil
		}

		visiting[project.Name] = true

		// Visit dependencies first
		for _, depName := range project.Dependencies {
			for _, depProject := range group.Projects {
				if depProject.Name == depName {
					if err := visit(depProject); err != nil {
						return err
					}
					break
				}
			}
		}

		visiting[project.Name] = false
		visited[project.Name] = true
		result = append(result, project)
		return nil
	}

	// Visit all projects
	for _, project := range group.Projects {
		if !visited[project.Name] {
			if err := visit(project); err != nil {
				return nil, err
			}
		}
	}

	return result, nil
}

func buildGroup(groupName string, verbose bool) error {
	group, err := loadGroup(groupName)
	if err != nil {
		return err
	}

	buildOrder, err := resolveBuildOrder(group)
	if err != nil {
		return fmt.Errorf("failed to resolve build order: %v", err)
	}

	fmt.Printf("🔨 Building group '%s' with %d projects\n", groupName, len(buildOrder))
	fmt.Printf("📋 Build order: ")
	for i, project := range buildOrder {
		if i > 0 {
			fmt.Printf(" -> ")
		}
		fmt.Printf("%s", project.Name)
	}
	fmt.Println()

	for _, project := range buildOrder {
		fmt.Printf("\n🏗️  Building project: %s\n", project.Name)

		// Copy assets from dependencies
		if err := copyDependencyAssets(group, project, verbose); err != nil {
			return fmt.Errorf("failed to copy dependency assets for %s: %v", project.Name, err)
		}

		// Build the project
		if err := buildProjectOnProject(project.Name, verbose); err != nil {
			return fmt.Errorf("failed to build project %s: %v", project.Name, err)
		}

		fmt.Printf("✅ Project %s built successfully\n", project.Name)
	}

	fmt.Printf("\n🎉 Group '%s' built successfully!\n", groupName)
	return nil
}

func listGroups() error {
	groupsDir := "groups"
	entries, err := os.ReadDir(groupsDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("No groups found.")
			return nil
		}
		return fmt.Errorf("failed to read groups directory: %v", err)
	}

	fmt.Println("Groups:")
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			groupName := strings.TrimSuffix(entry.Name(), ".json")
			group, err := loadGroup(groupName)
			if err != nil {
				fmt.Printf("  - %s (error loading)\n", groupName)
				continue
			}

			fmt.Printf("  - %s (%d projects): %s\n", groupName, len(group.Projects), group.Description)
			for _, project := range group.Projects {
				deps := ""
				if len(project.Dependencies) > 0 {
					deps = fmt.Sprintf(" [depends on: %s]", strings.Join(project.Dependencies, ", "))
				}
				fmt.Printf("    └─ %s%s\n", project.Name, deps)
			}
		}
	}
	return nil
}

func findProjectGroup(projectName string) (string, []string) {
	groupsDir := "groups"
	entries, err := os.ReadDir(groupsDir)
	if err != nil {
		return "", nil
	}

	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			groupName := strings.TrimSuffix(entry.Name(), ".json")
			group, err := loadGroup(groupName)
			if err != nil {
				continue
			}

			for _, project := range group.Projects {
				if project.Name == projectName {
					return groupName, project.Dependencies
				}
			}
		}
	}

	return "", nil
}

func copyDependencyAssets(group *Group, project GroupProject, verbose bool) error {
	if len(project.Dependencies) == 0 {
		return nil
	}

	projectPath := filepath.Join("projects", project.Name)

	for _, depName := range project.Dependencies {
		// Find the dependency project
		var depProject *GroupProject
		for i, p := range group.Projects {
			if p.Name == depName {
				depProject = &group.Projects[i]
				break
			}
		}

		if depProject == nil {
			continue
		}

		// Get the assets directory of the dependency
		depProjectPath := filepath.Join("projects", depName)
		depType := DetectProjectType(depProjectPath)
		depAssetsDir := getAssetsDirectory(depType, depProjectPath)

		if _, err := os.Stat(depAssetsDir); os.IsNotExist(err) {
			if verbose {
				fmt.Printf("⚠️  Dependency %s has no built assets yet\n", depName)
			}
			continue
		}

		// Determine target directory for assets
		var targetDir string
		if project.AssetDirs != nil {
			if customDir, exists := project.AssetDirs[depName]; exists {
				targetDir = filepath.Join(projectPath, customDir)
			} else {
				targetDir = filepath.Join(projectPath, "deps", depName)
			}
		} else {
			targetDir = filepath.Join(projectPath, "deps", depName)
		}

		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return fmt.Errorf("failed to create asset directory: %v", err)
		}

		// Copy assets
		if verbose {
			fmt.Printf("📦 Copying assets from %s to %s\n", depAssetsDir, targetDir)
		}

		if err := copyDir(depAssetsDir, targetDir); err != nil {
			return fmt.Errorf("failed to copy assets from %s: %v", depName, err)
		}
	}

	return nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Calculate relative path
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		targetPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(targetPath, info.Mode())
		}

		// Copy file
		return copyFile(path, targetPath)
	})
}

func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = destFile.ReadFrom(sourceFile)
	return err
}
func copyBuiltAssets(projectName string, projectType ProjectType, projectPath string, targetDir string) error {
	sourceDir := getAssetsDirectory(projectType, projectPath)
	if _, err := os.Stat(sourceDir); os.IsNotExist(err) {
		return fmt.Errorf("built assets not found in %s", sourceDir)
	}
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return fmt.Errorf("failed to create target directory: %v", err)
	}
	return copyDir(sourceDir, targetDir)
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
					&cli.StringFlag{
						Name:    "asset-dir",
						Aliases: []string{"o"},
						Usage:   "Directory to output built assets (defaults to project-specific directory)",
						Value:   "",
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
				Name:  "restart",
				Usage: "Restart a running project",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "verbose",
						Aliases: []string{"v"},
						Usage:   "Show verbose output",
					},
				},
				Action: restartProject,
			},
			{
				Name:  "create-group",
				Usage: "Create a new project group",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "name",
						Aliases:  []string{"n"},
						Usage:    "Group name",
						Required: true,
					},
					&cli.StringFlag{
						Name:    "description",
						Aliases: []string{"d"},
						Usage:   "Group description",
						Value:   "",
					},
				},
				Action: createGroupCmd,
			},
			{
				Name:  "add-to-group",
				Usage: "Add a project to a group with dependencies",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "group",
						Aliases:  []string{"g"},
						Usage:    "Group name",
						Required: true,
					},
					&cli.StringFlag{
						Name:     "project",
						Aliases:  []string{"p"},
						Usage:    "Project name",
						Required: true,
					},
					&cli.StringSliceFlag{
						Name:    "depends-on",
						Aliases: []string{"d"},
						Usage:   "Projects this project depends on",
					},
				},
				Action: addToGroupCmd,
			},
			{
				Name:  "build-group",
				Usage: "Build all projects in a group respecting dependencies",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "group",
						Aliases:  []string{"g"},
						Usage:    "Group name",
						Required: true,
					},
					&cli.BoolFlag{
						Name:    "verbose",
						Aliases: []string{"v"},
						Usage:   "Show verbose output",
					},
				},
				Action: buildGroupCmd,
			},
			{
				Name:   "list-groups",
				Usage:  "List all project groups",
				Action: listGroupsCmd,
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
	cloneOptions := &git.CloneOptions{
		URL:      repoURL,
		Progress: os.Stdout,
	}

	// Configure authentication for SSH URLs
	if strings.HasPrefix(repoURL, "git@") {
		// Try to use SSH authentication
		auth, err := getSSHAuth()
		if err != nil {
			// Fall back to trying without auth (might work if SSH key is in ssh-agent)
			fmt.Printf("Warning: SSH auth setup failed (%v), trying without authentication\n", err)
		} else {
			cloneOptions.Auth = auth
		}
	}

	_, err := git.PlainClone(projectPath, false, cloneOptions)
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

func buildProject(c *cli.Context) error {
	projectName := c.Args().First()
	if projectName == "" {
		return fmt.Errorf("project name is required")
	}

	verbose := c.Bool("verbose")
	assetDir := c.String("asset-dir")

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

	// Handle custom asset directory
	if assetDir != "" {
		err := copyBuiltAssets(projectName, projectType, absProjectPath, assetDir)
		if err != nil {
			return fmt.Errorf("failed to copy assets to %s: %v", assetDir, err)
		}
		fmt.Printf("Assets copied to: %s\n", assetDir)
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

func restartProject(c *cli.Context) error {
	projectName := c.Args().First()
	if projectName == "" {
		return fmt.Errorf("project name is required")
	}

	verbose := c.Bool("verbose")

	fmt.Printf("Restarting project %s...\n", projectName)

	// Stop the project
	stopProjectContainers(projectName)

	// Start the project again
	return runDevOnProject(projectName, verbose)
}

func createGroupCmd(c *cli.Context) error {
	name := c.String("name")
	description := c.String("description")

	if name == "" {
		return fmt.Errorf("group name is required")
	}

	return createGroup(name, description)
}

func addToGroupCmd(c *cli.Context) error {
	groupName := c.String("group")
	projectName := c.String("project")
	dependencies := c.StringSlice("depends-on")

	if groupName == "" {
		return fmt.Errorf("group name is required")
	}
	if projectName == "" {
		return fmt.Errorf("project name is required")
	}

	return addProjectToGroup(groupName, projectName, dependencies)
}

func buildGroupCmd(c *cli.Context) error {
	groupName := c.String("group")
	verbose := c.Bool("verbose")

	if groupName == "" {
		return fmt.Errorf("group name is required")
	}

	return buildGroup(groupName, verbose)
}

func listGroupsCmd(c *cli.Context) error {
	return listGroups()
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
	f, err := ParseGoMod(filepath.Join(projectPath, "go.mod"))
	if err == nil && f.GoVersion != "" {
		return f.GoVersion
	}
	return ""
}

// getSSHAuth configures SSH authentication for git operations
func getSSHAuth() (transport.AuthMethod, error) {
	// Try default SSH key locations
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	sshDir := filepath.Join(homeDir, ".ssh")
	keyPaths := []string{
		filepath.Join(sshDir, "id_rsa"),
		filepath.Join(sshDir, "id_ed25519"),
		filepath.Join(sshDir, "id_ecdsa"),
	}

	for _, keyPath := range keyPaths {
		if _, err := os.Stat(keyPath); err == nil {
			// Try to load the private key
			auth, err := ssh.NewPublicKeysFromFile("git", keyPath, "")
			if err == nil {
				return auth, nil
			}
		}
	}

	return nil, fmt.Errorf("no SSH key found in standard locations")
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
	Group        string            `json:"group,omitempty"`              // Group this project belongs to
	GroupDeps    []string          `json:"group_dependencies,omitempty"` // Dependencies within the group
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

	// Group routes
	http.HandleFunc("/api/groups", handleGroups)
	http.HandleFunc("/api/groups/create", handleCreateGroup)
	http.HandleFunc("/api/groups/add-project", handleAddToGroup)
	http.HandleFunc("/api/groups/build", handleBuildGroup)

	// Additional project operations
	http.HandleFunc("/api/projects/restart", handleRestart)
	http.HandleFunc("/api/projects/generate-assets", handleGenerateAssets)
	http.HandleFunc("/api/projects/serve", handleServe)

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

			// Find which group this project belongs to
			groupName, groupDeps := findProjectGroup(entry.Name())

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
				Group:        groupName,
				GroupDeps:    groupDeps,
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

// Group API handlers
func handleGroups(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != "GET" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Method not allowed"})
		return
	}

	groupsDir := "groups"
	entries, err := os.ReadDir(groupsDir)
	if err != nil {
		if os.IsNotExist(err) {
			json.NewEncoder(w).Encode(APIResponse{Success: true, Data: []Group{}})
			return
		}
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: err.Error()})
		return
	}

	var groups []Group
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			groupName := strings.TrimSuffix(entry.Name(), ".json")
			group, err := loadGroup(groupName)
			if err == nil {
				groups = append(groups, *group)
			}
		}
	}

	json.NewEncoder(w).Encode(APIResponse{Success: true, Data: groups})
}

func handleCreateGroup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Method not allowed"})
		return
	}

	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Invalid JSON"})
		return
	}

	if req.Name == "" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Group name is required"})
		return
	}

	err := createGroup(req.Name, req.Description)
	if err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(APIResponse{Success: true, Message: "Group created successfully"})
}

func handleAddToGroup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Method not allowed"})
		return
	}

	var req struct {
		Group        string   `json:"group"`
		Project      string   `json:"project"`
		Dependencies []string `json:"dependencies"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Invalid JSON"})
		return
	}

	if req.Group == "" || req.Project == "" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Group and project names are required"})
		return
	}

	err := addProjectToGroup(req.Group, req.Project, req.Dependencies)
	if err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(APIResponse{Success: true, Message: "Project added to group successfully"})
}

func handleBuildGroup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Method not allowed"})
		return
	}

	var req struct {
		Group   string `json:"group"`
		Verbose bool   `json:"verbose"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Invalid JSON"})
		return
	}

	if req.Group == "" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Group name is required"})
		return
	}

	err := buildGroup(req.Group, req.Verbose)
	if err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(APIResponse{Success: true, Message: "Group built successfully"})
}

func handleRestart(w http.ResponseWriter, r *http.Request) {
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

	// Stop the project
	stopProjectContainers(req.Project)

	// Start the project again
	err := runDevOnProject(req.Project, req.Verbose)
	if err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(APIResponse{Success: true, Message: "Project restarted successfully"})
}

func handleGenerateAssets(w http.ResponseWriter, r *http.Request) {
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
	if _, err := os.Stat(absProjectPath); os.IsNotExist(err) {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Project does not exist"})
		return
	}

	projectType := DetectProjectType(absProjectPath)
	image, assetCmd := getAssetConfig(projectType)

	containerName := fmt.Sprintf("sandbox-%s-assets", req.Project)

	dockerCmd := exec.Command("docker", "run", "--rm", "--name", containerName,
		"-v", fmt.Sprintf("%s:/app", absProjectPath), "-w", "/app", image)
	dockerCmd.Args = append(dockerCmd.Args, assetCmd...)

	output, err := dockerCmd.CombinedOutput()
	if err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: fmt.Sprintf("Failed to generate assets: %v\nOutput: %s", err, string(output))})
		return
	}

	json.NewEncoder(w).Encode(APIResponse{Success: true, Message: "Assets generated successfully"})
}

func handleServe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method != "POST" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Method not allowed"})
		return
	}

	var req struct {
		Project string `json:"project"`
		Port    string `json:"port,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Invalid JSON"})
		return
	}

	if req.Project == "" {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Project name is required"})
		return
	}

	port := req.Port
	if port == "" {
		port = "8080"
	}

	projectPath := filepath.Join("projects", req.Project)
	absProjectPath, err := filepath.Abs(projectPath)
	if err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: err.Error()})
		return
	}
	if _, err := os.Stat(absProjectPath); os.IsNotExist(err) {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: "Project does not exist"})
		return
	}

	projectType := DetectProjectType(absProjectPath)
	assetsDir := getAssetsDirectory(projectType, absProjectPath)

	if _, err := os.Stat(assetsDir); os.IsNotExist(err) {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: fmt.Sprintf("Built assets not found in %s. Run build first", assetsDir)})
		return
	}

	containerName := fmt.Sprintf("sandbox-%s-serve", req.Project)

	// Remove existing container if it exists
	exec.Command("docker", "rm", "-f", containerName).Run()

	dockerCmd := exec.Command("docker", "run", "-d", "--name", containerName,
		"-v", fmt.Sprintf("%s:/usr/share/nginx/html", assetsDir), "-p", fmt.Sprintf("%s:80", port),
		"nginx:alpine")

	err = dockerCmd.Run()
	if err != nil {
		json.NewEncoder(w).Encode(APIResponse{Success: false, Message: err.Error()})
		return
	}

	json.NewEncoder(w).Encode(APIResponse{Success: true, Message: fmt.Sprintf("Asset server started on port %s", port)})
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
