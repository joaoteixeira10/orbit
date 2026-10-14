package planner

import "strings"

// pt holds the Portuguese names of what templates create (lists, columns,
// first pages), so a workspace made in Portuguese starts in Portuguese. The
// UI's own strings are translated in the SPA.
var pt = map[string]string{
	"Assignments": "Trabalhos", "Exams": "Exames", "Projects": "Projetos",
	"Reminders": "Lembretes", "Shopping": "Compras", "Chores": "Tarefas da casa",
	"To do": "Por fazer", "Doing": "Em curso", "Done": "Feito",
	"Handed in": "Entregue", "Upcoming": "Próximos", "Studying": "A estudar",
	"Backlog": "Pendentes", "In progress": "Em execução", "Review": "Revisão",
	"To buy": "Comprar", "Bought": "Comprado",
	"Class notes": "Apontamentos", "Meeting notes": "Notas de reuniões",
	"Journal": "Diário", "House notes": "Notas da casa",
	"Welcome": "Bem-vindo",
}

// Localize returns name in locale (en when there is no translation).
func Localize(name, locale string) string {
	if strings.HasPrefix(strings.ToLower(locale), "pt") {
		if t, ok := pt[name]; ok {
			return t
		}
	}
	return name
}
