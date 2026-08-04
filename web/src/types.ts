// mirrors the JSON shapes served by internal/web's handlers -- keep these
// in sync with the Go response/request structs when the API changes

export interface Stats {
  chats_monitored: number
  messages_indexed: number
  today_matches: number
  bookmarks: number
  ignored: number
  can_send: boolean
}

export type ChatKind = 'group' | 'channel'

export interface Chat {
  telegram_id: number
  title: string
  username: string
  tag: string
  kind: ChatKind
  paused: boolean
  resume_enabled: boolean
  resume_interval_seconds: number
  resume_text: string
}

export interface ChatBroadcastConfig {
  enabled: boolean
  interval_seconds: number
  text: string
}

export interface ResumeText {
  text: string
}

export interface Compliance {
  min_delay_seconds: number
  max_per_hour: number
}
