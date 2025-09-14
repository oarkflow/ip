package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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
