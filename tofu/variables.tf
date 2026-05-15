variable "hostnames" {
  description = "Ambience hostnames that can complete the Microsoft login callback."
  type        = list(string)
  default = [
    "ambience.romaine.life",
    "ambience.dev.romaine.life",
    "*.ambience.dev.romaine.life",
  ]
}

variable "key_vault_name" {
  type    = string
  default = "romaine-kv"
}

variable "preview_slot_count" {
  description = "Number of Glimmung-managed ambience preview slot namespaces (ambience-slot-1..N) to federate to ambience-identity. Match Glimmung's slot pool size for this project; raise this and re-apply when the pool grows."
  type        = number
  default     = 5
}
