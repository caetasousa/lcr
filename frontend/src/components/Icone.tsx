// Ícones lineares no estilo Solar (traço 1.5, cantos arredondados),
// desenhados à mão para não trazer uma dependência inteira.
const caminhos = {
  documento: 'M14 3H8a3 3 0 0 0-3 3v12a3 3 0 0 0 3 3h8a3 3 0 0 0 3-3V8l-5-5Zm0 0v3a2 2 0 0 0 2 2h3M9 13h6M9 17h4',
  enviar: 'M12 16V4m0 0L7.5 8.5M12 4l4.5 4.5M4 15v2a3 3 0 0 0 3 3h10a3 3 0 0 0 3-3v-2',
  caneta: 'M4 20h4L19 9a2.83 2.83 0 0 0-4-4L4 16v4Zm9.5-13.5 4 4',
  escudo: 'M12 3 5 6v5c0 4.5 3 8.3 7 10 4-1.7 7-5.5 7-10V6l-7-3Zm-3 9 2 2 4-4',
  chave: 'M14.5 9.5a3.5 3.5 0 1 1-7 0 3.5 3.5 0 0 1 7 0Zm-.9 2.5L20 18.5M17 15.5l2-2',
  certificado: 'M4 6a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v8a2 2 0 0 1-2 2h-4M8 9h8M8 12h3m4 3.5a2.5 2.5 0 1 1-5 0 2.5 2.5 0 0 1 5 0Zm-4 2V21l1.5-1 1.5 1v-3.5',
  baixar: 'M12 4v12m0 0 4.5-4.5M12 16l-4.5-4.5M4 17v1a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-1',
  sair: 'M15 12H4m0 0 3.5-3.5M4 12l3.5 3.5M10 5V4a1 1 0 0 1 1-1h7a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-7a1 1 0 0 1-1-1v-1',
  esquerda: 'M15 18l-6-6 6-6',
  direita: 'M9 6l6 6-6 6',
  fechar: 'M6 6l12 12M18 6 6 18',
  check: 'M5 12.5 10 17l9-10',
  x: 'M7 7l10 10M17 7 7 17',
  alerta: 'M12 9v4m0 3.5v.5M10.3 4.2 3.2 17a2 2 0 0 0 1.7 3h14.2a2 2 0 0 0 1.7-3L13.7 4.2a2 2 0 0 0-3.4 0Z',
  cadeado: 'M7 10V7a5 5 0 0 1 10 0v3M6 10h12a1 1 0 0 1 1 1v8a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2v-8a1 1 0 0 1 1-1Zm6 4v3',
  rede: 'M12 5a2 2 0 1 0 0-4 2 2 0 0 0 0 4Zm0 0v5m0 0-6 5m6-5 6 5M6 21a2 2 0 1 0 0-4 2 2 0 0 0 0 4Zm12 0a2 2 0 1 0 0-4 2 2 0 0 0 0 4Z',
  olho: 'M2.5 12S6 5 12 5s9.5 7 9.5 7-3.5 7-9.5 7-9.5-7-9.5-7Zm9.5 3a3 3 0 1 0 0-6 3 3 0 0 0 0 6Z',
  lista: 'M8 6h12M8 12h12M8 18h12M4 6h.01M4 12h.01M4 18h.01',
  alvo: 'M12 21a9 9 0 1 0 0-18 9 9 0 0 0 0 18Zm0-5a4 4 0 1 0 0-8 4 4 0 0 0 0 8Zm0-4h.01',
} as const

export type NomeIcone = keyof typeof caminhos

export default function Icone({ nome, tamanho = 18, className }: { nome: NomeIcone; tamanho?: number; className?: string }) {
  return (
    <svg
      className={className}
      width={tamanho}
      height={tamanho}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.5}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
    >
      <path d={caminhos[nome]} />
    </svg>
  )
}
