package planner

import (
	"encoding/json"
	"strings"
)

type node struct {
	Type    string           `json:"type"`
	Attrs   map[string]any   `json:"attrs,omitempty"`
	Text    string           `json:"text,omitempty"`
	Marks   []map[string]any `json:"marks,omitempty"`
	Content []node           `json:"content,omitempty"`
}

func para(parts ...node) node { return node{Type: "paragraph", Content: parts} }
func txt(s string) node       { return node{Type: "text", Text: s} }
func bold(s string) node {
	return node{Type: "text", Text: s, Marks: []map[string]any{{"type": "bold"}}}
}
func heading(level int, s string) node {
	return node{Type: "heading", Attrs: map[string]any{"level": level}, Content: []node{txt(s)}}
}
func todo(s string, done bool) node {
	return node{Type: "taskItem", Attrs: map[string]any{"checked": done}, Content: []node{para(txt(s))}}
}

// WelcomeDoc is the first page of a new workspace: what the editor can do,
// as a checklist to try. Written in English or Portuguese.
func WelcomeDoc(workspace, locale string) string {
	pt := strings.HasPrefix(strings.ToLower(locale), "pt")
	t := func(en, p string) string {
		if pt {
			return p
		}
		return en
	}
	doc := node{Type: "doc", Content: []node{
		para(txt(t("This is "+workspace+"’s first page. Write anything here: notes, plans, a reading list. Type ",
			"Esta é a primeira página de "+workspace+". Escreva o que quiser: notas, planos, uma lista de leituras. Escreva ")),
			bold("/"), txt(t(" for headings, checklists, tables, quotes and more.",
				" para títulos, listas de verificação, tabelas, citações e mais."))),
		heading(2, t("Try it", "Experimente")),
		{Type: "taskList", Content: []node{
			todo(t("Open this page and tick me", "Abra esta página e marque-me"), true),
			todo(t("Type / on an empty line", "Escreva / numa linha vazia"), false),
			todo(t("Add a task in Tasks, then drag it on the Board", "Crie uma tarefa em Tarefas e arraste-a no Quadro"), false),
			todo(t("Give a task a due date and find it in the Calendar", "Dê uma data a uma tarefa e encontre-a no Calendário"), false),
			todo(t("Press ⌘K (Ctrl+K) to search everything", "Carregue em ⌘K (Ctrl+K) para procurar em tudo"), false),
		}},
		{Type: "blockquote", Content: []node{para(txt(t(
			"Workspace settings (the ⋯ next to its name) turn modules on and off and invite people.",
			"As definições do espaço (o ⋯ ao lado do nome) ligam e desligam módulos e convidam pessoas.")))}},
	}}
	b, _ := json.Marshal(doc)
	return string(b)
}
