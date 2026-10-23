// The icons a workspace or a task list can wear, by the name stored on the
// server (lucide's kebab-case names). A fixed set keeps the bundle small —
// importing all of lucide would defeat tree-shaking. An icon that isn't here
// (or an emoji) is rendered as text by <WsIcon>.
import type { Component } from 'vue'
import {
  Award, BookOpen, Bookmark, Briefcase, Calculator, Calendar, Camera, Car, ChefHat, Code, Coffee, Columns3, Dumbbell,
  FileCheck, FileText, Film, Flag, FlaskConical, Folder, FolderKanban, Gamepad2, Gift, Globe, GraduationCap, Heart,
  Home, House, Landmark, Laptop, Leaf, Lightbulb, ListChecks, Mountain, Music, NotebookPen, Palette, PawPrint, Pencil,
  Plane, Repeat, Rocket, ShoppingCart, Sparkles, Star, Stethoscope, Sun, Target, Timer, Trophy, Users, Wallet, Wrench,
  Bell, Baby, Bike, Brush, Building2, Clapperboard, Languages, Microscope, PiggyBank, Shirt, Sprout, Tent, Ticket,
  StickyNote, ChartGantt, ChartPie, GitBranch,
} from 'lucide-vue-next'

export const ICONS: Record<string, Component> = {
  'graduation-cap': GraduationCap, briefcase: Briefcase, heart: Heart, users: Users, sparkles: Sparkles,
  house: House, home: Home, 'book-open': BookOpen, 'notebook-pen': NotebookPen, pencil: Pencil, 'file-text': FileText,
  'file-check': FileCheck, folder: Folder, 'folder-kanban': FolderKanban, 'columns-3': Columns3, 'list-checks': ListChecks,
  calendar: Calendar, target: Target, trophy: Trophy, award: Award, star: Star, flag: Flag, rocket: Rocket,
  lightbulb: Lightbulb, code: Code, laptop: Laptop, wrench: Wrench, calculator: Calculator, 'flask-conical': FlaskConical,
  microscope: Microscope, languages: Languages, landmark: Landmark, 'building-2': Building2, globe: Globe, plane: Plane,
  car: Car, bike: Bike, mountain: Mountain, tent: Tent, sun: Sun, leaf: Leaf, sprout: Sprout, 'paw-print': PawPrint,
  baby: Baby, dumbbell: Dumbbell, stethoscope: Stethoscope, 'chef-hat': ChefHat, coffee: Coffee,
  'shopping-cart': ShoppingCart, shirt: Shirt, gift: Gift, wallet: Wallet, 'piggy-bank': PiggyBank, music: Music,
  film: Film, clapperboard: Clapperboard, 'gamepad-2': Gamepad2, camera: Camera, palette: Palette, brush: Brush,
  ticket: Ticket, bell: Bell, timer: Timer, repeat: Repeat, bookmark: Bookmark,
  'sticky-note': StickyNote, 'chart-gantt': ChartGantt, 'chart-pie': ChartPie, 'git-branch': GitBranch,
}

// The picker's order: a few rows by theme.
export const ICON_CHOICES = Object.keys(ICONS).filter((k) => k !== 'home')

// The route of each module of a workspace (the overview is the workspace itself).
export const MODULE_ROUTES: Record<string, string> = {
  home: 'ws', notes: 'ws-notes', pages: 'ws-pages', tasks: 'ws-tasks', board: 'ws-board',
  timeline: 'ws-timeline', charts: 'ws-charts', calendar: 'ws-calendar', courses: 'ws-courses', grades: 'ws-grades',
  whiteboards: 'ws-whiteboards', habits: 'ws-habits', focus: 'ws-focus', goals: 'ws-goals', journal: 'ws-journal', bookmarks: 'ws-bookmarks',
}

// Apple's system colours (light values; tiles keep them in dark mode too).
export const COLORS = ['#007AFF', '#5856D6', '#AF52DE', '#FF2D55', '#FF3B30', '#FF9500', '#FFCC00', '#34C759', '#30B0C7', '#5AC8FA', '#A2845E', '#8E8E93']
