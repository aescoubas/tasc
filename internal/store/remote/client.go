package remote

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
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
	data, err := json.Marshal(t)
	if err != nil {
		return 0, err
	}

	resp, err := c.http.Post(c.baseURL+"/tasks", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		return 0, fmt.Errorf("server error: %s", resp.Status)
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
		return models.Task{}, fmt.Errorf("server error: %s", resp.Status)
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
		return fmt.Errorf("server error: %s", resp.Status)
	}
	return nil
}

func (c *Client) BatchUpdateTasks(ids []int64, updates map[string]interface{}) error {
	reqBody := map[string]interface{}{
		"ids":     ids,
		"updates": updates,
	}
	data, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	resp, err := c.http.Post(c.baseURL+"/tasks/batch-update", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server error: %s", resp.Status)
	}
	return nil
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
		return fmt.Errorf("server error: %s", resp.Status)
	}
	return nil
}

func (c *Client) ListTasks(filter store.TaskFilter) ([]models.Task, error) {
	u, err := url.Parse(c.baseURL + "/tasks")
	if err != nil {
		return nil, err
	}

	q := u.Query()
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
	// Status handling simplified for now
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

func (c *Client) MarkDone(id int64) error {
	resp, err := c.http.Post(fmt.Sprintf("%s/tasks/%d/done", c.baseURL, id), "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to mark done: %s", resp.Status)
	}
	return nil
}

func (c *Client) StartTask(id int64) error {
	resp, err := c.http.Post(fmt.Sprintf("%s/tasks/%d/start", c.baseURL, id), "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to start: %s", resp.Status)
	}
	return nil
}

func (c *Client) StopTask(id int64) error {
	resp, err := c.http.Post(fmt.Sprintf("%s/tasks/stop", c.baseURL), "application/json", nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to stop: %s", resp.Status)
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
		return fmt.Errorf("failed: %s", resp.Status)
	}
	return nil
}

func (c *Client) GetDependencies() ([]models.TaskDependency, error) {
	resp, err := c.http.Get(c.baseURL + "/dependencies")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

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

	var projects []models.Project
	if err := json.NewDecoder(resp.Body).Decode(&projects); err != nil {
		return nil, err
	}
	return projects, nil
}

func (c *Client) GetProject(name string) (models.Project, error) {
	resp, err := c.http.Get(fmt.Sprintf("%s/projects/%s", c.baseURL, url.PathEscape(name)))
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

func (c *Client) CreateProject(p models.Project) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	resp, err := c.http.Post(c.baseURL+"/projects", "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("failed: %s", resp.Status)
	}
	return nil
}

func (c *Client) UpdateProject(oldName string, p models.Project) error {
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/projects/%s", c.baseURL, url.PathEscape(oldName)), bytes.NewBuffer(data))
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
	req, err := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/projects/%s", c.baseURL, url.PathEscape(name)), nil)
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
