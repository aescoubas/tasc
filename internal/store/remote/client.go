package remote

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/aescoubas/tasc/internal/models"
	"github.com/aescoubas/tasc/internal/store"
)

type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{},
	}
}

func (c *Client) CreateTask(t models.Task) (int64, error) {
	return c.createTask("/tasks", t)
}

func (c *Client) CreateTaskWithLinks(t models.Task, links store.TaskLinks) (int64, int, error) {
	if links.Empty() {
		id, err := c.CreateTask(t)
		return id, 0, err
	}
	var result struct {
		ID    int64 `json:"id"`
		Added int   `json:"links_added"`
	}
	err := c.postTaskWithLinks("/tasks/with-links", http.StatusCreated, map[string]any{"task": t, "links": links}, &result)
	return result.ID, result.Added, err
}

func (c *Client) LogTask(t models.Task) (int64, error) {
	return c.createTask("/tasks/log", t)
}

func (c *Client) createTask(path string, t models.Task) (int64, error) {
	data, err := json.Marshal(t)
	if err != nil {
		return 0, err
	}

	resp, err := c.http.Post(c.baseURL+path, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return 0, taskResponseError(resp)
	}

	var created models.Task
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return 0, err
	}
	return created.ID, nil
}

func (c *Client) GetTask(id int64) (models.Task, error) {
	resp, err := c.http.Get(fmt.Sprintf("%s/tasks/%d", c.baseURL, id))
	if err != nil {
		return models.Task{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return models.Task{}, taskResponseError(resp)
	}

	var t models.Task
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return models.Task{}, err
	}
	return t, nil
}

func (c *Client) UpdateTask(t models.Task) error {
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/tasks/%d", c.baseURL, t.ID), bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return taskResponseError(resp)
	}
	return nil
}

func (c *Client) UpdateTaskDescription(id int64, expected, description string) error {
	data, err := json.Marshal(map[string]string{"expected": expected, "description": description})
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/tasks/%d/description", c.baseURL, id), bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return taskResponseError(resp)
	}
	return nil
}

func (c *Client) BatchUpdateTasks(ids []int64, updates map[string]interface{}) error {
	path := "/tasks/batch-update"
	if value, ok := updates["user_priority"]; ok {
		if err := store.ValidateUserPriority(value); err != nil {
			return err
		}
		// Older servers ignore unknown batch fields. Use a dedicated route
		// so they reject the whole edit instead of silently dropping priority.
		path += "/priority"
	}
	if _, keep := updates["keep_series"]; keep {
		path = "/tasks/batch-update/keep-series"
	}
	reqBody := map[string]interface{}{
		"ids":     ids,
		"updates": updates,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	resp, err := c.http.Post(c.baseURL+path, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return taskResponseError(resp)
	}
	return nil
}

func (c *Client) BatchUpdateTasksWithLinks(ids []int64, updates map[string]any, links store.TaskLinks) (int, error) {
	if links.Empty() {
		return 0, c.BatchUpdateTasks(ids, updates)
	}
	var result struct {
		Added int `json:"links_added"`
	}
	err := c.postTaskWithLinks("/tasks/batch-update/with-links", http.StatusOK,
		map[string]any{"ids": ids, "updates": updates, "links": links}, &result)
	return result.Added, err
}

// Dedicated routes ensure older servers reject the entire operation.
func (c *Client) postTaskWithLinks(path string, wantStatus int, body, result any) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	resp, err := c.http.Post(c.baseURL+path, "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		return taskResponseError(resp)
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

func (c *Client) DeleteTask(id int64) error {
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/tasks/%d", c.baseURL, id), nil)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return taskResponseError(resp)
	}
	return nil
}

func (c *Client) ListTasks(filter store.TaskFilter) ([]models.Task, error) {
	u, err := url.Parse(c.baseURL + "/tasks")
	if err != nil {
		return nil, err
	}

	q := u.Query()
	if len(filter.Status) > 0 {
		statuses := make([]string, len(filter.Status))
		for i, status := range filter.Status {
			statuses[i] = string(status)
		}
		q.Set("status", strings.Join(statuses, ","))
	}
	if filter.IncludeDeleted {
		q.Set("include_deleted", "true")
	}
	if filter.ExcludeArchivedProjects {
		q.Set("exclude_archived_projects", "true")
	}
	if len(filter.Projects) > 0 {
		q.Set("projects", strings.Join(filter.Projects, ","))
	}
	if len(filter.IDs) > 0 {
		var ids []string
		for _, id := range filter.IDs {
			ids = append(ids, fmt.Sprintf("%d", id))
		}
		q.Set("ids", strings.Join(ids, ","))
	}
	if filter.DueBefore != nil {
		q.Set("due_before", filter.DueBefore.Format(time.RFC3339))
	}
	if filter.DueAfter != nil {
		q.Set("due_after", filter.DueAfter.Format(time.RFC3339))
	}
	u.RawQuery = q.Encode()

	resp, err := c.http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, taskResponseError(resp)
	}

	var tasks []models.Task
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}

func (c *Client) MarkDone(id int64) error {
	resp, err := c.http.Post(fmt.Sprintf("%s/tasks/%d/done", c.baseURL, id), "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return taskResponseError(resp)
	}
	return nil
}

func (c *Client) MarkVague(id int64) error {
	resp, err := c.http.Post(fmt.Sprintf("%s/tasks/%d/vague", c.baseURL, id), "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return taskResponseError(resp)
	}
	return nil
}

func taskResponseError(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("server error: %s (reading error: %w)", resp.Status, err)
	}
	message := strings.TrimSpace(string(body))
	switch resp.StatusCode {
	case http.StatusNotFound:
		return fmt.Errorf("%s: %w", strings.TrimSuffix(message, ": "+sql.ErrNoRows.Error()), sql.ErrNoRows)
	case http.StatusBadRequest:
		for _, category := range []error{store.ErrInvalidCompletion, store.ErrInvalidUserPriority, store.ErrInvalidDependency, store.ErrInvalidTask} {
			if message == category.Error() {
				return category
			}
			if detail, ok := strings.CutPrefix(message, category.Error()+": "); ok {
				return fmt.Errorf("%w: %s", category, detail)
			}
		}
		return fmt.Errorf("%w: %s", store.ErrInvalidTask, message)
	case http.StatusConflict:
		switch message {
		case store.ErrTaskDescriptionConflict.Error():
			return store.ErrTaskDescriptionConflict
		case store.AssignmentConflict:
			return errors.New(store.AssignmentConflict)
		}
	}
	return fmt.Errorf("server error: %s: %s", resp.Status, message)
}

func (c *Client) StartTask(id int64) error {
	resp, err := c.http.Post(fmt.Sprintf("%s/tasks/%d/start", c.baseURL, id), "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return taskResponseError(resp)
	}
	return nil
}

func (c *Client) StopTask(id *int64) error {
	endpoint := c.baseURL + "/tasks/stop"
	if id != nil {
		if *id == -1 {
			// Older servers treat ?id=-1 as an omitted target.
			endpoint = c.baseURL + "/tasks/-1/stop"
		} else {
			endpoint += "?id=" + strconv.FormatInt(*id, 10)
		}
	}
	resp, err := c.http.Post(endpoint, "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return taskResponseError(resp)
	}
	return nil
}

func (c *Client) GetActiveTask() (*models.Task, error) {
	resp, err := c.http.Get(c.baseURL + "/tasks/active")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // No active task
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server error: %s", resp.Status)
	}

	var t models.Task
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return nil, err
	}
	return &t, nil
}

func (c *Client) AddDependency(blockerID, blockedID int64) error {
	reqBody := map[string]int64{"blocker_id": blockerID, "blocked_id": blockedID}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	resp, err := c.http.Post(c.baseURL+"/dependencies", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return taskResponseError(resp)
	}
	return nil
}

func (c *Client) RemoveDependency(blockerID, blockedID int64) error {
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/dependencies/%d/%d", c.baseURL, blockerID, blockedID), nil)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return taskResponseError(resp)
	}
	return nil
}

func (c *Client) GetDependencies() ([]models.TaskDependency, error) {
	resp, err := c.http.Get(c.baseURL + "/dependencies")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, taskResponseError(resp)
	}

	var deps []models.TaskDependency
	if err := json.NewDecoder(resp.Body).Decode(&deps); err != nil {
		return nil, err
	}
	return deps, nil
}

func (c *Client) ListProjects() ([]models.Project, error) {
	resp, err := c.http.Get(c.baseURL + "/projects")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, taskResponseError(resp)
	}

	var projects []models.Project
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (c *Client) GetProject(name string) (models.Project, error) {
	resp, err := c.http.Get(c.projectURL(name))
	if err != nil {
		return models.Project{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return models.Project{}, fmt.Errorf("server error: %s", resp.Status)
	}

	var p models.Project
	if err := json.NewDecoder(resp.Body).Decode(&p); err != nil {
		return models.Project{}, err
	}
	return p, nil
}

func (c *Client) CreateProject(p models.Project, options store.ProjectCreateOptions) error {
	path := "/projects"
	if options != (store.ProjectCreateOptions{}) {
		// Older servers must reject a request that depends on options they do not implement.
		path += "/create"
	}
	data, err := json.Marshal(struct {
		models.Project
		Options store.ProjectCreateOptions `json:"options,omitzero"`
	}{p, options})
	if err != nil {
		return err
	}
	resp, err := c.http.Post(c.baseURL+path, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return projectResponseError(resp)
	}
	return nil
}

func (c *Client) UpdateProject(oldName string, p models.Project) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, c.projectURL(oldName), bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed: %s", resp.Status)
	}
	return nil
}

func (c *Client) DeleteProject(name string) error {
	req, err := http.NewRequest(http.MethodDelete, c.projectURL(name), nil)
	if err != nil {
		return err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed: %s", resp.Status)
	}
	return nil
}

func (c *Client) ArchiveProject(name string) error {
	resp, err := c.http.Post(c.projectURL(name)+"/archive", "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return projectResponseError(resp)
	}
	return nil
}

func (c *Client) DeleteProjectWithMode(name string, mode store.ProjectDeleteMode) (store.ProjectDeleteResult, error) {
	var result store.ProjectDeleteResult
	body, err := json.Marshal(map[string]store.ProjectDeleteMode{"mode": mode})
	if err != nil {
		return result, err
	}
	resp, err := c.http.Post(c.projectURL(name)+"/delete", "application/json", bytes.NewReader(body))
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return result, projectResponseError(resp)
	}
	err = json.NewDecoder(resp.Body).Decode(&result)
	return result, err
}

func (c *Client) projectURL(name string) string {
	segment := url.PathEscape(name)
	if segment == "." || segment == ".." {
		segment = strings.ReplaceAll(segment, ".", "%2E")
	}
	return c.baseURL + "/projects/" + segment
}

func projectResponseError(resp *http.Response) error {
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return fmt.Errorf("server error: %s (reading error: %w)", resp.Status, err)
	}
	message := strings.TrimSpace(string(body))
	switch resp.StatusCode {
	case http.StatusNotFound:
		return fmt.Errorf("%w: %s", sql.ErrNoRows, message)
	case http.StatusConflict:
		return fmt.Errorf("%w: %s", store.ErrProjectNotEmpty, message)
	default:
		return fmt.Errorf("server error: %s: %s", resp.Status, message)
	}
}

func (c *Client) SearchTasks(query string) ([]models.Task, error) {
	u, _ := url.Parse(c.baseURL + "/search")
	q := u.Query()
	q.Set("q", query)
	u.RawQuery = q.Encode()

	resp, err := c.http.Get(u.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server error: %s", resp.Status)
	}

	var tasks []models.Task
	if err := json.NewDecoder(resp.Body).Decode(&tasks); err != nil {
		return nil, err
	}
	return tasks, nil
}
