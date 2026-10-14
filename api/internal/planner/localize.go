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
	"Welcome":     "Bem-vindo",
	"Quick notes": "Notas rápidas",
	NoteTip:       "Aponte aqui o que não pode esquecer. Fixe o que importa, dê cores por tema e converta uma nota numa página ou em tarefas.",
}

// NoteTip is the text of the note a new workspace with quick notes starts with.
const NoteTip = "Jot down what you can't forget. Pin what matters, colour notes by topic, and turn a note into a page or into tasks."

// Localize returns name in locale (en when there is no translation).
func Localize(name, locale string) string {
	if strings.HasPrefix(strings.ToLower(locale), "pt") {
		if t, ok := pt[name]; ok {
			return t
		}
	}
	return name
}
