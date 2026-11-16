// Cliente da API Go. A sessão vai em um cookie HttpOnly (o JavaScript não o lê).

export class ApiError extends Error {
  status: number
  constructor(msg: string, status: number) {
    super(msg)
    this.status = status
  }
}

async function req(path: string, init?: RequestInit): Promise<Response> {
  const r = await fetch(path, { credentials: 'same-origin', ...init })
  if (!r.ok) {
    let msg = `Erro ${r.status}`
    try {
      msg = (await r.json()).erro ?? msg
    } catch {
      /* resposta sem JSON */
    }
    throw new ApiError(msg, r.status)
  }
  return r
}

const json = (body: unknown): RequestInit => ({
  method: 'POST',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(body),
})

export interface Certificado {
  titular: string
  emissor: string
  serial: string
  valido_de: string
  valido_ate: string
  algoritmo: string
  tamanho_chave: number
  fingerprint_sha256: string
  usos_chave: string[]
  lcr: string[]
  revogado_em?: string
  motivo_revogacao?: string
  situacao: 'válido' | 'revogado' | 'expirado'
  pem: string
}

export interface Eu {
  nome: string
  cpf: string
  certificado: Certificado
}

export interface CertInfo {
  titular: string
  emissor: string
  serial: string
  valido_de: string
  valido_ate: string
}

export interface Assinatura {
  nome: string
  motivo?: string
  local?: string
  contato?: string
  data_assinatura?: string
  integra: boolean
  cadeia_confiavel: boolean
  revogado_na_lcr: boolean
  certificado?: CertInfo
  cadeia: CertInfo[]
  erros: string[]
  avisos: string[]
  lcr: {
    emitido_por_esta_ac: boolean
    numero_lcr: string
    revogado_em: string | null
    motivo?: string
    revogado_antes_da_assinatura: boolean
  }
  valida: boolean
}

export interface ResultadoValidacao {
  paginas: number
  sha256: string
  assinaturas: Assinatura[]
  erro?: string
  valido: boolean
}

export interface Documento {
  id: number
  nome: string
  pagina: number
  assinado_em: string
  sha256_original: string
  sha256_assinado: string
  tamanho: number
  signatarios: string[]
}

export interface InfoAC {
  raiz: Record<string, string>
  intermediaria: Record<string, string>
  revogados: { serial: string; titular: string; revogado_em: string; motivo: string }[]
}

export interface Posicao {
  pagina: number
  x1: number
  y1: number
  x2: number
  y2: number
}

export interface PdfAssinado {
  blob: Blob
  nome: string
  id: string
  sha256Original: string
  sha256Assinado: string
}

export const api = {
  eu: async (): Promise<Eu | null> => {
    try {
      return await (await req('/api/eu')).json()
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) return null
      throw e
    }
  },
  login: (cpf: string, senha: string) => req('/api/login', json({ cpf, senha })),
  cadastro: (nome: string, cpf: string, senha: string) => req('/api/cadastro', json({ nome, cpf, senha })),
  logout: () => req('/api/logout', { method: 'POST' }),

  assinar: async (arquivo: File, pos: Posicao, senha: string, motivo: string): Promise<PdfAssinado> => {
    const f = new FormData()
    f.append('arquivo', arquivo)
    f.append('pagina', String(pos.pagina))
    f.append('x1', pos.x1.toFixed(2))
    f.append('y1', pos.y1.toFixed(2))
    f.append('x2', pos.x2.toFixed(2))
    f.append('y2', pos.y2.toFixed(2))
    f.append('senha', senha)
    f.append('motivo', motivo)
    const r = await req('/api/assinar', { method: 'POST', body: f })
    return {
      blob: await r.blob(),
      nome: arquivo.name.replace(/\.pdf$/i, '') + '_assinado.pdf',
      id: r.headers.get('X-Documento-Id') ?? '',
      sha256Original: r.headers.get('X-SHA256-Original') ?? '',
      sha256Assinado: r.headers.get('X-SHA256-Assinado') ?? '',
    }
  },

  validar: async (arquivo: Blob, nome = 'documento.pdf'): Promise<ResultadoValidacao> => {
    const f = new FormData()
    f.append('arquivo', arquivo, nome)
    return (await req('/api/validar', { method: 'POST', body: f })).json()
  },

  documentos: async (): Promise<Documento[]> => (await req('/api/documentos')).json(),
  // O PDF fica cifrado no servidor: a senha libera a chave privada que o decifra.
  documento: async (id: number, senha: string): Promise<Blob> => (await req(`/api/documentos/${id}`, json({ senha }))).blob(),

  exportarPfx: async (senha: string): Promise<Blob> => (await req('/api/certificado/pfx', json({ senha }))).blob(),
  revogar: (senha: string, motivo: number) => req('/api/certificado/revogar', json({ senha, motivo })),
  renovar: (senha: string) => req('/api/certificado/renovar', json({ senha })),

  ac: async (): Promise<InfoAC> => (await req('/api/ac')).json(),
}

export function baixar(blob: Blob, nome: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = nome
  a.click()
  setTimeout(() => URL.revokeObjectURL(url), 1000)
}

export const dataHora = (s?: string | null) =>
  s ? new Date(s).toLocaleString('pt-BR', { dateStyle: 'short', timeStyle: 'medium' }) : '—'

export const mascaraCPF = (v: string) => {
  const d = v.replace(/\D/g, '').slice(0, 11)
  return d
    .replace(/^(\d{3})(\d)/, '$1.$2')
    .replace(/^(\d{3})\.(\d{3})(\d)/, '$1.$2.$3')
    .replace(/\.(\d{3})(\d{1,2})$/, '.$1-$2')
}
