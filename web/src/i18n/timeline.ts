// The Timeline module (tasks on a Gantt chart).
export const tlEn = {
  timeline: {
    title: 'Timeline',
    zoom: 'Zoom',
    zooms: { weeks: 'Weeks', months: 'Months', quarters: 'Quarters' },
    tasksCol: 'Tasks',
    count: '{n} task | {n} tasks',
    unscheduled: '{n} without dates',
    overdue: '{n} overdue',
    noDate: 'No date',
    scheduleOn: 'Schedule on {date}',
    days: '{n} day | {n} days',
    collapse: 'Collapse {name}',
    expand: 'Expand {name}',
    readOnly: 'View only',
    noLists: 'No lists yet',
    noListsHint: 'Create a list in Tasks, then plan its tasks here.',
    barHint: 'Drag to reschedule, drag an edge to change the start or due date',
    moved: 'Moved to {dates}',
  },
}

export const tlPt: typeof tlEn = {
  timeline: {
    title: 'Cronograma',
    zoom: 'Zoom',
    zooms: { weeks: 'Semanas', months: 'Meses', quarters: 'Trimestres' },
    tasksCol: 'Tarefas',
    count: '{n} tarefa | {n} tarefas',
    unscheduled: '{n} sem data',
    overdue: '{n} em atraso',
    noDate: 'Sem data',
    scheduleOn: 'Agendar para {date}',
    days: '{n} dia | {n} dias',
    collapse: 'Recolher {name}',
    expand: 'Expandir {name}',
    readOnly: 'Só leitura',
    noLists: 'Ainda sem listas',
    noListsHint: 'Crie uma lista em Tarefas e planeie aqui as suas tarefas.',
    barHint: 'Arraste para reagendar, arraste uma ponta para mudar o início ou o prazo',
    moved: 'Movida para {dates}',
  },
}
