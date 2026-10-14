package planner

import "strings"

// WelcomeDoc is the first page of a new workspace (Markdown): what the editor
// can do, as a checklist to try. Written in English or Portuguese.
func WelcomeDoc(workspace, locale string) string {
	pt := strings.HasPrefix(strings.ToLower(locale), "pt")
	t := func(en, p string) string {
		if pt {
			return p
		}
		return en
	}
	todo := func(done bool, en, p string) string {
		box := "[ ]"
		if done {
			box = "[x]"
		}
		return "- " + box + " " + Escape(t(en, p))
	}
	return strings.Join([]string{
		Escape(t("This is "+workspace+"’s first page. Write anything here: notes, plans, a reading list. Type ",
			"Esta é a primeira página de "+workspace+". Escreva o que quiser: notas, planos, uma lista de leituras. Escreva ")) +
			"**/**" + t(" for headings, checklists, tables, quotes and more.", " para títulos, listas de verificação, tabelas, citações e mais."),
		"",
		"## " + t("Try it", "Experimente"),
		"",
		todo(true, "Open this page and tick me", "Abra esta página e marque-me"),
		todo(false, "Type / on an empty line", "Escreva / numa linha vazia"),
		todo(false, "Add a task in Tasks, then drag it on the Board", "Crie uma tarefa em Tarefas e arraste-a no Quadro"),
		todo(false, "Give a task a due date and find it in the Calendar", "Dê uma data a uma tarefa e encontre-a no Calendário"),
		todo(false, "Press ⌘K (Ctrl+K) to search everything", "Carregue em ⌘K (Ctrl+K) para procurar em tudo"),
		"",
		"> " + Escape(t("Workspace settings (the ⋯ next to its name) turn modules on and off and invite people.",
			"As definições do espaço (o ⋯ ao lado do nome) ligam e desligam módulos e convidam pessoas.")),
	}, "\n")
}
