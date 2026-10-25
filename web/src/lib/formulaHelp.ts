// What a formula editor suggests: the functions (lib/formula.ts) with
// their arguments and a line on what each does, in English and Portuguese
// (Portuguese Excel's names), and where the caret is in a formula — the
// name being typed, and the function call it's in (for the argument hint).

export interface FnHelp {
  en: string // Excel's name
  pt: string // Portuguese Excel's
  args: [string, string] // the arguments, as Excel lists them (en, pt)
  desc: [string, string] // what it does (en, pt)
}

const F = (en: string, pt: string, argsEn: string, argsPt: string, descEn: string, descPt: string): FnHelp => ({ en, pt, args: [argsEn, argsPt], desc: [descEn, descPt] })

export const FUNCTIONS: FnHelp[] = [
  F('SUM', 'SOMA', 'number1, [number2], …', 'núm1, [núm2], …', 'Adds numbers up', 'Soma números'),
  F('AVERAGE', 'MÉDIA', 'number1, [number2], …', 'núm1, [núm2], …', 'The average of the numbers', 'A média dos números'),
  F('MIN', 'MÍNIMO', 'number1, [number2], …', 'núm1, [núm2], …', 'The smallest number', 'O número mais pequeno'),
  F('MAX', 'MÁXIMO', 'number1, [number2], …', 'núm1, [núm2], …', 'The largest number', 'O número maior'),
  F('COUNT', 'CONTAR', 'value1, [value2], …', 'valor1, [valor2], …', 'How many are numbers or dates', 'Quantos são números ou datas'),
  F('COUNTA', 'CONTAR.VAL', 'value1, [value2], …', 'valor1, [valor2], …', 'How many aren’t empty', 'Quantos não estão vazios'),
  F('COUNTBLANK', 'CONTAR.VAZIO', 'range', 'intervalo', 'How many are empty', 'Quantos estão vazios'),
  F('COUNTIF', 'CONTAR.SE', 'range, criteria', 'intervalo, critérios', 'How many meet a condition (">5", "done")', 'Quantos cumprem uma condição (">5", "feito")'),
  F('COUNTIFS', 'CONTAR.SE.S', 'range1, criteria1, [range2, criteria2], …', 'intervalo1, critérios1, [intervalo2, critérios2], …', 'How many meet every condition', 'Quantos cumprem todas as condições'),
  F('SUMIF', 'SOMA.SE', 'range, criteria, [sum_range]', 'intervalo, critérios, [intervalo_soma]', 'Adds those that meet a condition', 'Soma os que cumprem uma condição'),
  F('SUMIFS', 'SOMA.SE.S', 'sum_range, range1, criteria1, …', 'intervalo_soma, intervalo1, critérios1, …', 'Adds those that meet every condition', 'Soma os que cumprem todas as condições'),
  F('AVERAGEIF', 'MÉDIA.SE', 'range, criteria, [average_range]', 'intervalo, critérios, [intervalo_média]', 'The average of those that meet a condition', 'A média dos que cumprem uma condição'),
  F('AVERAGEIFS', 'MÉDIA.SE.S', 'average_range, range1, criteria1, …', 'intervalo_média, intervalo1, critérios1, …', 'The average of those that meet every condition', 'A média dos que cumprem todas as condições'),
  F('PRODUCT', 'PRODUTO', 'number1, [number2], …', 'núm1, [núm2], …', 'Multiplies numbers', 'Multiplica números'),
  F('MEDIAN', 'MED', 'number1, [number2], …', 'núm1, [núm2], …', 'The middle number', 'O número do meio'),
  F('ROUND', 'ARRED', 'number, digits', 'núm, núm_dígitos', 'Rounds to a number of decimals', 'Arredonda a um número de casas decimais'),
  F('ROUNDUP', 'ARRED.PARA.CIMA', 'number, digits', 'núm, núm_dígitos', 'Rounds up, away from zero', 'Arredonda para cima'),
  F('ROUNDDOWN', 'ARRED.PARA.BAIXO', 'number, digits', 'núm, núm_dígitos', 'Rounds down, towards zero', 'Arredonda para baixo'),
  F('INT', 'INT', 'number', 'núm', 'The whole number at or below', 'O inteiro igual ou abaixo'),
  F('TRUNC', 'TRUNCAR', 'number, [digits]', 'núm, [núm_dígitos]', 'Cuts the decimals off', 'Corta as casas decimais'),
  F('CEILING', 'ARRED.EXCESSO', 'number, significance', 'núm, significância', 'Rounds up to a multiple', 'Arredonda para cima a um múltiplo'),
  F('FLOOR', 'ARRED.DEFEITO', 'number, significance', 'núm, significância', 'Rounds down to a multiple', 'Arredonda para baixo a um múltiplo'),
  F('ABS', 'ABS', 'number', 'núm', 'The number without its sign', 'O número sem sinal'),
  F('SIGN', 'SINAL', 'number', 'núm', '1, 0 or -1', '1, 0 ou -1'),
  F('MOD', 'RESTO', 'number, divisor', 'núm, divisor', 'What’s left after dividing', 'O resto da divisão'),
  F('POWER', 'POTÊNCIA', 'number, power', 'núm, potência', 'A number raised to a power', 'Um número elevado a uma potência'),
  F('SQRT', 'RAIZQ', 'number', 'núm', 'The square root', 'A raiz quadrada'),
  F('PI', 'PI', '', '', 'π, 3.14159…', 'π, 3,14159…'),
  F('LN', 'LN', 'number', 'núm', 'The natural logarithm', 'O logaritmo natural'),
  F('LOG', 'LOG', 'number, [base]', 'núm, [base]', 'The logarithm (base 10 by default)', 'O logaritmo (de base 10 se não disser)'),
  F('LOG10', 'LOG10', 'number', 'núm', 'The logarithm in base 10', 'O logaritmo de base 10'),
  F('EXP', 'EXP', 'number', 'núm', 'e raised to a power', 'e elevado a uma potência'),
  F('IF', 'SE', 'condition, value_if_true, [value_if_false]', 'teste_lógico, valor_se_verdadeiro, [valor_se_falso]', 'One value or another, by a condition', 'Um valor ou outro, conforme uma condição'),
  F('IFS', 'SE.S', 'condition1, value1, [condition2, value2], …', 'teste1, valor1, [teste2, valor2], …', 'The value of the first condition that holds', 'O valor da primeira condição verdadeira'),
  F('IFERROR', 'SE.ERRO', 'value, value_if_error', 'valor, valor_se_erro', 'A value, or another when it’s an error', 'Um valor, ou outro quando dá erro'),
  F('AND', 'E', 'logical1, [logical2], …', 'lógico1, [lógico2], …', 'TRUE when all are true', 'VERDADEIRO quando todos são verdadeiros'),
  F('OR', 'OU', 'logical1, [logical2], …', 'lógico1, [lógico2], …', 'TRUE when any is true', 'VERDADEIRO quando algum é verdadeiro'),
  F('NOT', 'NÃO', 'logical', 'lógico', 'The opposite', 'O contrário'),
  F('CHOOSE', 'SELECCIONAR', 'index, value1, [value2], …', 'núm_índice, valor1, [valor2], …', 'The value at a position', 'O valor numa posição'),
  F('ISBLANK', 'É.CÉL.VAZIA', 'value', 'valor', 'TRUE when empty', 'VERDADEIRO quando vazio'),
  F('ISNUMBER', 'É.NÚM', 'value', 'valor', 'TRUE when a number', 'VERDADEIRO quando é um número'),
  F('ISTEXT', 'É.TEXTO', 'value', 'valor', 'TRUE when text', 'VERDADEIRO quando é texto'),
  F('ISERROR', 'É.ERRO', 'value', 'valor', 'TRUE when an error', 'VERDADEIRO quando é um erro'),
  F('VLOOKUP', 'PROCV', 'lookup_value, table, col_index, [approximate]', 'valor_proc, matriz_tabela, núm_índice_coluna, [procurar_intervalo]', 'Finds a row by its first column and gives another column', 'Procura uma linha pela primeira coluna e dá outra coluna'),
  F('HLOOKUP', 'PROCH', 'lookup_value, table, row_index', 'valor_proc, matriz_tabela, núm_índice_linha', 'Finds a column by its first row and gives another row', 'Procura uma coluna pela primeira linha e dá outra linha'),
  F('XLOOKUP', 'PROCX', 'lookup_value, lookup_range, return_range, [if_not_found]', 'valor_pesquisa, matriz_pesquisa, matriz_devolver, [se_não_encontrado]', 'Finds a value and gives the one beside it', 'Procura um valor e dá o que está ao lado'),
  F('INDEX', 'ÍNDICE', 'range, row, [column]', 'matriz, núm_linha, [núm_coluna]', 'The value at a row and column', 'O valor numa linha e coluna'),
  F('MATCH', 'CORRESP', 'lookup_value, range, [match_type]', 'valor_proc, matriz_proc, [tipo_corresp]', 'Where a value is (0: exactly)', 'Onde está um valor (0: exato)'),
  F('CONCAT', 'CONCATENAR', 'text1, [text2], …', 'texto1, [texto2], …', 'Joins texts', 'Junta textos'),
  F('TEXTJOIN', 'UNIR.TEXTO', 'delimiter, ignore_empty, text1, …', 'delimitador, ignorar_vazio, texto1, …', 'Joins texts with a separator', 'Junta textos com um separador'),
  F('LEN', 'NÚM.CARAT', 'text', 'texto', 'How many characters', 'Quantos caracteres'),
  F('LEFT', 'ESQUERDA', 'text, [count]', 'texto, [núm_caract]', 'The first characters', 'Os primeiros caracteres'),
  F('RIGHT', 'DIREITA', 'text, [count]', 'texto, [núm_caract]', 'The last characters', 'Os últimos caracteres'),
  F('MID', 'SEG.TEXTO', 'text, start, count', 'texto, núm_inicial, núm_caract', 'Characters from the middle', 'Caracteres do meio'),
  F('UPPER', 'MAIÚSCULAS', 'text', 'texto', 'In capitals', 'Em maiúsculas'),
  F('LOWER', 'MINÚSCULAS', 'text', 'texto', 'In small letters', 'Em minúsculas'),
  F('PROPER', 'INICIAL.MAIÚSCULA', 'text', 'texto', 'Each Word Capitalised', 'Cada Palavra Com Maiúscula'),
  F('TRIM', 'COMPACTAR', 'text', 'texto', 'Without extra spaces', 'Sem espaços a mais'),
  F('SUBSTITUTE', 'SUBST', 'text, old, new', 'texto, texto_antigo, novo_texto', 'Replaces some text with other', 'Troca um texto por outro'),
  F('REPT', 'REPETIR', 'text, times', 'texto, núm_vezes', 'Repeats a text', 'Repete um texto'),
  F('FIND', 'PROCURAR', 'find_text, within_text, [start]', 'texto_a_localizar, no_texto, [núm_inicial]', 'Where a text is (case matters)', 'Onde está um texto (com maiúsculas)'),
  F('SEARCH', 'LOCALIZAR', 'find_text, within_text', 'texto_a_localizar, no_texto', 'Where a text is (case aside)', 'Onde está um texto (sem olhar a maiúsculas)'),
  F('VALUE', 'VALOR', 'text', 'texto', 'A text as a number', 'Um texto como número'),
  F('TODAY', 'HOJE', '', '', 'Today’s date', 'A data de hoje'),
  F('DATE', 'DATA', 'year, month, day', 'ano, mês, dia', 'A date from its parts', 'Uma data a partir das partes'),
  F('YEAR', 'ANO', 'date', 'data', 'A date’s year', 'O ano de uma data'),
  F('MONTH', 'MÊS', 'date', 'data', 'A date’s month (1–12)', 'O mês de uma data (1–12)'),
  F('DAY', 'DIA', 'date', 'data', 'A date’s day of the month', 'O dia do mês de uma data'),
  F('WEEKDAY', 'DIA.SEMANA', 'date', 'data', 'The day of the week (Sunday 1)', 'O dia da semana (domingo 1)'),
  F('DAYS', 'DIAS', 'end_date, start_date', 'data_final, data_inicial', 'Days between two dates', 'Dias entre duas datas'),
  F('DATEDIF', 'DATADIF', 'start_date, end_date, unit', 'data_inicial, data_final, unidade', 'Time between dates in "D", "M" or "Y"', 'Tempo entre datas em "D", "M" ou "Y"'),
  F('EDATE', 'DATAM', 'start_date, months', 'data_inicial, meses', 'A date some months later', 'Uma data uns meses depois'),
  F('EOMONTH', 'FIM.MÊS', 'start_date, months', 'data_inicial, meses', 'The month’s last day, some months later', 'O último dia do mês, uns meses depois'),
  F('NETWORKDAYS', 'DIATRABALHOTOTAL', 'start_date, end_date', 'data_inicial, data_final', 'Weekdays between two dates', 'Dias úteis entre duas datas'),
]

const fold = (s: string) => s.normalize('NFD').replace(/[̀-ͯ]/g, '').toLowerCase()
const byName = new Map<string, FnHelp>()
for (const f of FUNCTIONS) {
  byName.set(fold(f.en), f)
  byName.set(fold(f.pt), f)
}
export const helpFor = (name: string) => byName.get(fold(name))

// suggest: the functions whose name (English or Portuguese) starts with
// what's typed, the language's own first; then those that contain it.
export function suggest(prefix: string, pt: boolean, max = 8): FnHelp[] {
  const p = fold(prefix)
  if (!p) return []
  const name = (f: FnHelp) => fold(pt ? f.pt : f.en)
  const other = (f: FnHelp) => fold(pt ? f.en : f.pt)
  const starts = FUNCTIONS.filter((f) => name(f).startsWith(p))
  const otherStarts = FUNCTIONS.filter((f) => !starts.includes(f) && other(f).startsWith(p))
  const has = FUNCTIONS.filter((f) => !starts.includes(f) && !otherStarts.includes(f) && name(f).includes(p))
  return [...starts, ...otherStarts, ...has].slice(0, max)
}

// at reads a formula up to the caret: the name being typed (when it's a
// function's place) and the call the caret is in, with which argument.
export function at(text: string, caret: number): { word: { from: number; text: string } | null; call: { name: string; arg: number } | null } {
  const s = text.slice(0, caret)
  const stack: { name: string; arg: number }[] = []
  let inStr = false
  let inRef = false
  for (let i = 0; i < s.length; i++) {
    const ch = s[i]
    if (inStr) {
      if (ch === '"') inStr = s[i + 1] === '"' ? (i++, true) : false
      continue
    }
    if (inRef) {
      if (ch === ']') inRef = false
      continue
    }
    if (ch === '"') inStr = true
    else if (ch === '[') inRef = true
    else if (ch === '(') {
      const m = /([\p{L}_][\p{L}\p{N}_.]*)\s*$/u.exec(s.slice(0, i))
      stack.push({ name: m ? m[1] : '', arg: 0 })
    } else if (ch === ')') stack.pop()
    else if ((ch === ',' || ch === ';') && stack.length) stack[stack.length - 1].arg++
  }
  if (inStr || inRef) return { word: null, call: stack.at(-1) ?? null }
  // A name being typed where a function can go: after the start, an
  // operator, "(" or a separator — not after a sheet's "!" or a number.
  const m = /(^|[=(,;+\-*/^&<>\s])([\p{L}_][\p{L}\p{N}_.]*)$/u.exec(s)
  const word = m && !/^\$?[A-Za-z]{1,3}\$?\d+$/.test(m[2]) ? { from: s.length - m[2].length, text: m[2] } : null
  const call = [...stack].reverse().find((c) => c.name && helpFor(c.name)) ?? null
  return { word, call }
}
