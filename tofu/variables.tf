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
  description = "Ambience-owned Key Vault for app secrets projected into Kubernetes."
  type        = string
  default     = "ng6-ambience"
}
