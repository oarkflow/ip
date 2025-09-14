package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/urfave/cli/v2"
)

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
