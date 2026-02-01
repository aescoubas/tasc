import React from 'react';
import useSWR from 'swr';
import { Table, Badge, Spinner, Alert } from 'react-bootstrap';
import { fetchTasks } from '../api';
import { Task } from '../types';

const TaskList: React.FC = () => {
  const { data: tasks, error, isLoading } = useSWR<Task[]>('tasks', fetchTasks);

  if (isLoading) return <div className="text-center p-5"><Spinner animation="border" /></div>;
  if (error) return <Alert variant="danger">Error loading tasks</Alert>;

  return (
    <Table striped bordered hover responsive>
      <thead>
        <tr>
          <th>ID</th>
          <th>Project</th>
          <th>Description</th>
          <th>Status</th>
          <th>Due</th>
        </tr>
      </thead>
      <tbody>
        {tasks?.map((task) => (
          <tr key={task.id}>
            <td>{task.id}</td>
            <td>{task.project}</td>
            <td>
                {task.is_blocked && <span className="me-2">🚫</span>}
                {task.description}
            </td>
            <td>
              <Badge bg={getStatusColor(task.status)}>{task.status}</Badge>
            </td>
            <td>{task.due_at ? new Date(task.due_at).toLocaleDateString() : '-'}</td>
          </tr>
        ))}
      </tbody>
    </Table>
  );
};

function getStatusColor(status: string) {
  switch (status) {
    case 'done': return 'success';
    case 'ongoing': return 'primary';
    case 'blocked': return 'danger';
    case 'backlog': return 'secondary';
    default: return 'secondary';
  }
}

export default TaskList;
