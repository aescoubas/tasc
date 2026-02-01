import React from 'react';
import { Card, Badge } from 'react-bootstrap';
import { useSortable } from '@dnd-kit/sortable';
import { CSS } from '@dnd-kit/utilities';
import { Task } from '../types';

interface Props {
  task: Task;
}

const TaskCard: React.FC<Props> = ({ task }) => {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
  } = useSortable({ id: task.id });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
    marginBottom: '0.5rem',
    cursor: 'grab',
  };

  return (
    <div ref={setNodeRef} style={style} {...attributes} {...listeners}>
      <Card>
        <Card.Body className="p-2">
          <div className="d-flex justify-content-between align-items-start mb-1">
            <small className="text-muted">#{task.id}</small>
            {task.project && <Badge bg="light" text="dark">{task.project}</Badge>}
          </div>
          <Card.Text className="mb-1">
            {task.is_blocked && "🚫 "}
            {task.description}
          </Card.Text>
          {task.due_at && (
             <small className={new Date(task.due_at) < new Date() ? 'text-danger' : 'text-muted'}>
               Due: {new Date(task.due_at).toLocaleDateString()}
             </small>
          )}
        </Card.Body>
      </Card>
    </div>
  );
};

export default TaskCard;
