package inbox

import (
	"fmt"
	"strings"
)

// The wording of notifications sent from the server (push, channels). The
// app has the same sentences in its own catalogue (web/src/i18n/inbox.ts);
// keep the two in step.

type phrase struct {
	title, body string
	// titleMany / bodyMany are used when a group counts more than one ({n}).
	titleMany, bodyMany string
}

var phrases = map[string]map[string]phrase{
	"en": {
		"test":          {title: "Notifications are on", body: "This device will get Orbit's notifications."},
		"task_due":      {title: "{title}", body: "Due now · {ws}"},
		"event_now":     {title: "{title}", body: "Starting now · {ws}"},
		"event_soon":    {title: "{title}", body: "In {lead} · {ws}"},
		"task_assigned": {title: "{actor} assigned you a task", body: "{title} · {ws}"},
		"added":         {title: "{actor} added you to {ws}", body: "As {role}"},
		"role_changed":  {title: "Your role in {ws} changed", body: "{actor} made you {role}"},
		"removed":       {title: "You were removed from {ws}", body: "By {actor}"},
		"task_added":    {title: "{actor} added a task", body: "{title} · {ws}", titleMany: "{actor} added {n} tasks", bodyMany: "The latest: {title} · {ws}"},
		"task_done":     {title: "{actor} completed a task", body: "{title} · {ws}", titleMany: "{actor} completed {n} tasks", bodyMany: "The latest: {title} · {ws}"},
		"page_added":    {title: "{actor} added a page", body: "{title} · {ws}", titleMany: "{actor} added {n} pages", bodyMany: "The latest: {title} · {ws}"},
		"note_added":    {title: "{actor} added a note", body: "{title} · {ws}", titleMany: "{actor} added {n} notes", bodyMany: "The latest: {title} · {ws}"},
		"event_added":   {title: "{actor} added an event", body: "{title} · {ws}", titleMany: "{actor} added {n} events", bodyMany: "The latest: {title} · {ws}"},
		"member_joined": {title: "{actor} joined {ws}", body: "As {role}"},
		"task_comment":  {title: "{actor} commented on {title}", body: "{excerpt}", titleMany: "{n} comments on {title}", bodyMany: "The latest, by {actor}: {excerpt}"},
		"mentioned":     {title: "{actor} mentioned you", body: "{title}: {excerpt}"},
		"daily":         {title: "Your day: {summary}", body: "{lines}"},
		"held":          {title: "{n} notifications while it was quiet", body: "{lines}"},
		"note_reminder": {title: "{title}", body: "Note · {ws}"},
	},
	"pt": {
		"test":          {title: "As notificações estão ligadas", body: "Este dispositivo vai receber as notificações do Orbit."},
		"task_due":      {title: "{title}", body: "Para agora · {ws}"},
		"event_now":     {title: "{title}", body: "A começar agora · {ws}"},
		"event_soon":    {title: "{title}", body: "Daqui a {lead} · {ws}"},
		"task_assigned": {title: "{actor} atribuiu-lhe uma tarefa", body: "{title} · {ws}"},
		"added":         {title: "{actor} deu-lhe acesso a {ws}", body: "Como {role}"},
		"role_changed":  {title: "O seu papel em {ws} mudou", body: "Agora: {role} (por {actor})"},
		"removed":       {title: "Já não tem acesso a {ws}", body: "Por {actor}"},
		"task_added":    {title: "{actor} adicionou uma tarefa", body: "{title} · {ws}", titleMany: "{actor} adicionou {n} tarefas", bodyMany: "A mais recente: {title} · {ws}"},
		"task_done":     {title: "{actor} concluiu uma tarefa", body: "{title} · {ws}", titleMany: "{actor} concluiu {n} tarefas", bodyMany: "A mais recente: {title} · {ws}"},
		"page_added":    {title: "{actor} adicionou uma página", body: "{title} · {ws}", titleMany: "{actor} adicionou {n} páginas", bodyMany: "A mais recente: {title} · {ws}"},
		"note_added":    {title: "{actor} adicionou uma nota", body: "{title} · {ws}", titleMany: "{actor} adicionou {n} notas", bodyMany: "A mais recente: {title} · {ws}"},
		"event_added":   {title: "{actor} adicionou um evento", body: "{title} · {ws}", titleMany: "{actor} adicionou {n} eventos", bodyMany: "O mais recente: {title} · {ws}"},
		"member_joined": {title: "{actor} entrou em {ws}", body: "Como {role}"},
		"task_comment":  {title: "{actor} comentou {title}", body: "{excerpt}", titleMany: "{n} comentários em {title}", bodyMany: "O mais recente, de {actor}: {excerpt}"},
		"mentioned":     {title: "{actor} mencionou-o", body: "{title}: {excerpt}"},
		"daily":         {title: "O seu dia: {summary}", body: "{lines}"},
		"held":          {title: "{n} notificações durante o silêncio", body: "{lines}"},
		"note_reminder": {title: "{title}", body: "Nota · {ws}"},
	},
}

var roleNames = map[string]map[string]string{
	"en": {"owner": "owner", "editor": "editor", "viewer": "viewer"},
	"pt": {"owner": "proprietário", "editor": "editor", "viewer": "leitor"},
}

// lang is "pt" for any Portuguese locale, else "en".
func lang(locale string) string {
	if strings.HasPrefix(strings.ToLower(locale), "pt") {
		return "pt"
	}
	return "en"
}

// Render words a notification in a language: its title and body.
func Render(locale string, v View) (title, body string) {
	l := lang(locale)
	ph, ok := phrases[l][v.Verb]
	if !ok {
		ph = phrase{title: v.Verb}
	}
	vars := map[string]string{"n": fmt.Sprint(v.Count)}
	for k, val := range v.Params {
		vars[k] = fmt.Sprint(val)
	}
	if r, ok := roleNames[l][vars["role"]]; ok {
		vars["role"] = r
	}
	if v.Verb == "daily" {
		vars["summary"] = digestSummary(l, v.Params)
		vars["lines"] = digestLines(v.Params)
	}
	t, b := ph.title, ph.body
	if v.Count > 1 && ph.titleMany != "" {
		t, b = ph.titleMany, ph.bodyMany
	}
	return fill(t, vars), strings.TrimSuffix(fill(b, vars), " · ")
}

func fill(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

func num(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case float64:
		return int(n)
	}
	return 0
}

// digestSummary is "3 tasks, 2 events" (overdue ones counted among tasks).
func digestSummary(l string, p map[string]any) string {
	tasks, events := num(p["tasks"]), num(p["events"])
	plural := func(n int, one, many string) string {
		if n == 1 {
			return "1 " + one
		}
		return fmt.Sprintf("%d %s", n, many)
	}
	if l == "pt" {
		return plural(tasks, "tarefa", "tarefas") + ", " + plural(events, "evento", "eventos")
	}
	return plural(tasks, "task", "tasks") + ", " + plural(events, "event", "events")
}

// digestLines lists the day: events with their time, then tasks (⚠ when
// overdue). Symbols, not words, so they need no translating.
func digestLines(p map[string]any) string {
	raw, _ := p["items"].([]any)
	lines := make([]string, 0, len(raw))
	for _, r := range raw {
		it, _ := r.(map[string]any)
		title, _ := it["t"].(string)
		at, _ := it["at"].(string)
		switch it["k"] {
		case "event":
			if at != "" {
				title = at + " " + title
			}
			lines = append(lines, "◷ "+title)
		case "overdue":
			lines = append(lines, "⚠ "+title)
		default:
			if at != "" {
				title += " (" + at + ")"
			}
			lines = append(lines, "○ "+title)
		}
	}
	return strings.Join(lines, "\n")
}

// pings are what the shared channels get by default: what kind of thing
// happened, never what it's about (no titles, names or workspaces).
var pings = map[string]map[string]string{
	"en": {
		Reminder:  "A reminder for you",
		Assigned:  "Someone gave you a task",
		Comment:   "A new comment or mention for you",
		Workspace: "A change in your workspaces",
		Activity:  "New activity in a shared workspace",
		Digest:    "Your daily summary is ready",
		"":        "You have a new notification",
		"open":    "Open Orbit to see it",
	},
	"pt": {
		Reminder:  "Um lembrete para si",
		Assigned:  "Alguém lhe atribuiu uma tarefa",
		Comment:   "Um novo comentário ou menção para si",
		Workspace: "Uma alteração nos seus espaços",
		Activity:  "Nova atividade num espaço partilhado",
		Digest:    "O seu resumo do dia está pronto",
		"":        "Tem uma nova notificação",
		"open":    "Abra o Orbit para a ver",
	},
}

// Ping is the title and body of a notification for a shared channel: its
// kind only, and a link to the app (when its address is known).
func Ping(locale, kind, appURL string) (title, body string) {
	words := pings[lang(locale)]
	t, ok := words[kind]
	if !ok {
		t = words[""]
	}
	body = words["open"]
	if appURL != "" {
		body += ": " + appURL
	}
	return "Orbit: " + t, body
}
