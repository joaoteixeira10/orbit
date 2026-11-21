// The privacy policy and terms pages (/privacy, /terms), and the built-in
// privacy text shown until the owner writes their own (Owner tab). Portuguese
// is European Portuguese. No braces other than {app}, no at-signs: these are
// vue-i18n messages.
export const legalEn = {
  legal: {
    privacy: 'Privacy',
    terms: 'Terms',
    privacyTitle: 'Privacy policy',
    termsTitle: 'Terms of service',
    back: 'Back to {app}',
    signIn: 'Sign in',
    missing: 'This page isn’t there.',
    card: {
      title: 'Privacy and terms',
      hint: 'Shown to anyone at /privacy and /terms, and linked from the sign-in page. Google asks for a privacy policy before people can sync their Google Calendar. In Markdown.',
      privacy: 'Privacy policy',
      privacyHint: 'Empty: Orbit’s own text, in each reader’s language.',
      useDefault: 'Start from Orbit’s text',
      terms: 'Terms of service',
      termsHint: 'Empty: no terms page, and no link to it.',
      open: 'Open the page',
      saved: 'Privacy and terms saved',
    },
    defaultPrivacy: `{app} is a planner run by the people who manage this site. This page says what it keeps about you, and what it does with it.

## What is kept

- **Your account:** your name, email address, password (kept only as a one-way hash), two-factor settings and preferences.
- **What you put in:** your tasks, pages, notes, events, files and comments, in the workspaces you belong to. The members of a workspace see what is in it.
- **Sign-ins and activity:** when you sign in and from which address, and a log of changes, to keep the service secure.

## Accounts you connect

When you connect another service, {app} keeps the access you give it, encrypted, and uses it only for what you asked:

- **Google Calendar:** to read and write the calendar you pick, keeping it in step with your workspace’s calendar.
- **Git servers** (GitHub, GitLab, Gitea, Forgejo): to keep the issues of the repositories you link in step with their tasks.
- **Other calendars** (iCloud, Nextcloud, subscribed calendars): to sync or show their events.

You can disconnect any of them at any time: Stop syncing in {app}, or remove its access in that service.

{app}’s use and transfer of information received from Google APIs adheres to the [Google API Services User Data Policy](https://developers.google.com/terms/api-services-user-data-policy), including the Limited Use requirements.

## What is not done

- Your data is not sold, rented or used for advertising.
- It is not shared with anyone outside this site, except the services you connect yourself and, only when you use the built-in assistant, the AI model this site uses for it, which receives what you ask and what it needs to answer.

## Cookies

Only what signing in needs. No tracking or advertising cookies.

## Your choices

You can change or delete what you put in at any time. To have your account and its data deleted, or to ask what is kept about you, contact the people who run this site.`,
  },
}

export const legalPt = {
  legal: {
    privacy: 'Privacidade',
    terms: 'Termos',
    privacyTitle: 'Política de privacidade',
    termsTitle: 'Termos de serviço',
    back: 'Voltar ao {app}',
    signIn: 'Iniciar sessão',
    missing: 'Esta página não existe.',
    card: {
      title: 'Privacidade e termos',
      hint: 'Visíveis para qualquer pessoa em /privacy e /terms, com ligação na página de início de sessão. A Google pede uma política de privacidade antes de as pessoas poderem sincronizar o Google Calendar. Em Markdown.',
      privacy: 'Política de privacidade',
      privacyHint: 'Vazia: o texto do Orbit, na língua de quem lê.',
      useDefault: 'Começar pelo texto do Orbit',
      terms: 'Termos de serviço',
      termsHint: 'Vazios: não há página de termos, nem ligação para ela.',
      open: 'Abrir a página',
      saved: 'Privacidade e termos guardados',
    },
    defaultPrivacy: `O {app} é um planeador gerido pelas pessoas responsáveis por este site. Esta página diz o que guarda sobre si e o que faz com isso.

## O que é guardado

- **A sua conta:** o seu nome, endereço de email, palavra-passe (guardada apenas como hash irreversível), definições de autenticação de dois fatores e preferências.
- **O que lá coloca:** as suas tarefas, páginas, notas, eventos, ficheiros e comentários, nos espaços de trabalho a que pertence. Os membros de um espaço de trabalho veem o que lá está.
- **Inícios de sessão e atividade:** quando inicia sessão e a partir de que endereço, e um registo das alterações, para manter o serviço seguro.

## Contas que liga

Quando liga outro serviço, o {app} guarda o acesso que lhe dá, cifrado, e usa-o apenas para o que pediu:

- **Google Calendar:** para ler e escrever o calendário que escolher, mantendo-o a par do calendário do seu espaço de trabalho.
- **Servidores git** (GitHub, GitLab, Gitea, Forgejo): para manter os issues dos repositórios que ligar a par das respetivas tarefas.
- **Outros calendários** (iCloud, Nextcloud, calendários subscritos): para sincronizar ou mostrar os seus eventos.

Pode desligar qualquer um deles a qualquer momento: Parar de sincronizar no {app}, ou retirar-lhe o acesso nesse serviço.

A utilização e transferência, pelo {app}, de informação recebida das APIs da Google cumpre a [Google API Services User Data Policy](https://developers.google.com/terms/api-services-user-data-policy), incluindo os requisitos de Utilização Limitada.

## O que não é feito

- Os seus dados não são vendidos, alugados nem usados para publicidade.
- Não são partilhados com ninguém fora deste site, exceto os serviços que liga e, apenas quando usa o assistente integrado, o modelo de IA que este site usa para isso, que recebe o que pergunta e o que precisa para responder.

## Cookies

Apenas os necessários para iniciar sessão. Nenhum cookie de rastreio ou de publicidade.

## As suas escolhas

Pode alterar ou apagar o que lá coloca a qualquer momento. Para apagar a sua conta e os respetivos dados, ou para saber o que é guardado sobre si, contacte as pessoas que gerem este site.`,
  },
}
