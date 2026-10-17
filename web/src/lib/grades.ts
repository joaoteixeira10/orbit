// Grades: weighted averages and what the next mark must be. Marks can be out
// of anything (12/20, 80/100); they count as fractions, shown on the
// workspace's scale. The server works out the same averages.
import type { Grade } from '@/api/planner'

type Mark = Pick<Grade, 'score' | 'out_of' | 'weight'>

// average is the weighted average as a fraction (0..1), or null with no weight.
export function average(marks: Mark[]): number | null {
  let sum = 0
  let w = 0
  for (const g of marks) {
    if (g.out_of <= 0 || g.weight <= 0) continue
    sum += (g.score / g.out_of) * g.weight
    w += g.weight
  }
  return w ? sum / w : null
}

// needed is what the next mark (of weight w) must be, as a fraction of its
// maximum, for the average to reach target (a fraction): (target·(W+w) − S)/w.
// Above 1: out of reach; 0 or less: any mark will do.
export function needed(marks: Mark[], target: number, w: number): number {
  if (w <= 0) return 0
  let sum = 0
  let total = 0
  for (const g of marks) {
    if (g.out_of <= 0 || g.weight <= 0) continue
    sum += (g.score / g.out_of) * g.weight
    total += g.weight
  }
  return (target * (total + w) - sum) / w
}

// fmtMark shows a mark on a scale: 14.5, 15, 87.3 (one decimal at most).
export function fmtMark(v: number, locale?: string): string {
  return new Intl.NumberFormat(locale, { maximumFractionDigits: 1 }).format(Math.round(v * 10) / 10)
}

// passes: at least half the scale.
export const passes = (fraction: number) => fraction >= 0.5 - 1e-9
