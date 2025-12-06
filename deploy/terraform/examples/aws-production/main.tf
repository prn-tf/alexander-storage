# Production AWS Deployment Example
# This example deploys Alexander Storage with PostgreSQL, Redis, and HA configuration

terraform {
  required_version = ">= 1.0"
  
  # Uncomment for remote state
  # backend "s3" {
  #   bucket = "your-terraform-state-bucket"
  #   key    = "alexander/production/terraform.tfstate"
  #   region = "us-east-1"
  # }
}

provider "aws" {
  region = var.region
  
  default_tags {
    tags = {
      Environment = var.environment
      ManagedBy   = "terraform"
      Project     = "alexander-storage"
    }
  }
}

# Variables
variable "region" {
  default = "us-east-1"
}

variable "environment" {
  default = "production"
}

variable "vpc_id" {
  description = "VPC ID for deployment"
  type        = string
}

variable "private_subnet_ids" {
  description = "Private subnet IDs for deployment"
  type        = list(string)
}

variable "public_subnet_ids" {
  description = "Public subnet IDs for ALB"
  type        = list(string)
}

variable "domain_name" {
  description = "Domain name for Alexander (e.g., s3.example.com)"
  type        = string
}

variable "certificate_arn" {
  description = "ACM certificate ARN for SSL"
  type        = string
}

variable "db_password" {
  description = "RDS database password (keep secure!)"
  type        = string
  sensitive   = true
  default     = ""
}

resource "random_password" "rds_password" {
  length  = 32
  special = true
}

# KMS Key for encryption
resource "aws_kms_key" "rds" {
  description             = "KMS key for Alexander RDS encryption"
  deletion_window_in_days = 10
  enable_key_rotation     = true
  
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid    = "Enable IAM policies"
        Effect = "Allow"
        Principal = {
          AWS = "arn:aws:iam::${data.aws_caller_identity.current.account_id}:root"
        }
        Action   = "kms:*"
        Resource = "*"
      },
      {
        Sid    = "Allow RDS"
        Effect = "Allow"
        Principal = {
          Service = "rds.amazonaws.com"
        }
        Action = [
          "kms:Decrypt",
          "kms:GenerateDataKey",
          "kms:CreateGrant"
        ]
        Resource = "*"
      }
    ]
  })
  
  tags = {
    Name = "alexander-${var.environment}-rds-key"
  }
}

resource "aws_kms_alias" "rds" {
  name          = "alias/alexander-${var.environment}-rds"
  target_key_id = aws_kms_key.rds.key_id
}

data "aws_caller_identity" "current" {}

# RDS PostgreSQL
module "postgresql" {
  source  = "terraform-aws-modules/rds/aws"
  version = "~> 6.0"
  
  identifier = "alexander-${var.environment}"
  
  engine               = "postgres"
  engine_version       = "15"
  family               = "postgres15"
  major_engine_version = "15"
  instance_class       = "db.r6g.large"
  
  allocated_storage     = 100
  max_allocated_storage = 500
  
  db_name  = "alexander"
  username = "alexander"
  password = var.db_password != "" ? var.db_password : random_password.rds_password.result
  port     = 5432
  
  multi_az               = true
  db_subnet_group_name   = aws_db_subnet_group.alexander.name
  vpc_security_group_ids = [aws_security_group.rds.id]
  
  # SECURITY: Enable encryption at rest
  storage_encrypted = true
  kms_key_id        = aws_kms_key.rds.arn
  
  # SECURITY: Enable encryption in transit
  ssl_mode = "require"
  
  maintenance_window      = "Mon:00:00-Mon:03:00"
  backup_window           = "03:00-06:00"
  backup_retention_period = 30
  
  deletion_protection = true
  
  performance_insights_enabled          = true
  performance_insights_retention_period = 7
  performance_insights_kms_key_id       = aws_kms_key.rds.arn
  
  create_cloudwatch_log_group     = true
  enabled_cloudwatch_logs_exports = ["postgresql", "upgrade"]
  
  parameters = [
    {
      name  = "shared_preload_libraries"
      value = "pg_stat_statements"
    }
  ]
  
  tags = {
    Name = "alexander-${var.environment}-db"
  }
}

resource "aws_db_subnet_group" "alexander" {
  name       = "alexander-${var.environment}"
  subnet_ids = var.private_subnet_ids
  
  tags = {
    Name = "alexander-${var.environment}-db-subnet"
  }
}

resource "aws_security_group" "rds" {
  name        = "alexander-${var.environment}-rds-sg"
  description = "Security group for Alexander RDS"
  vpc_id      = var.vpc_id
  
  ingress {
    description     = "PostgreSQL from Alexander"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [module.alexander.security_group_id]
  }
  
  tags = {
    Name = "alexander-${var.environment}-rds-sg"
  }
}

# ElastiCache Redis
module "redis" {
  source  = "terraform-aws-modules/elasticache/aws"
  version = "~> 1.0"
  
  cluster_id           = "alexander-${var.environment}"
  engine               = "redis"
  node_type            = "cache.r6g.large"
  num_cache_nodes      = 2
  parameter_group_name = "default.redis7"
  engine_version       = "7.0"
  port                 = 6379
  
  subnet_ids         = var.private_subnet_ids
  security_group_ids = [aws_security_group.redis.id]
  
  automatic_failover_enabled = true
  multi_az_enabled           = true
  
  at_rest_encryption_enabled = true
  transit_encryption_enabled = true
  
  maintenance_window = "tue:06:30-tue:07:30"
  snapshot_window    = "05:00-06:00"
  
  tags = {
    Name = "alexander-${var.environment}-redis"
  }
}

resource "aws_security_group" "redis" {
  name        = "alexander-${var.environment}-redis-sg"
  description = "Security group for Alexander Redis"
  vpc_id      = var.vpc_id
  
  ingress {
    description     = "Redis from Alexander"
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [module.alexander.security_group_id]
  }
  
  tags = {
    Name = "alexander-${var.environment}-redis-sg"
  }
}

# Alexander Storage
module "alexander" {
  source = "../../modules/aws"
  
  name        = "alexander"
  environment = var.environment
  region      = var.region
  
  vpc_id     = var.vpc_id
  subnet_ids = var.private_subnet_ids
  
  # High availability configuration
  instance_type    = "t3.large"
  min_size         = 3
  max_size         = 10
  desired_capacity = 3
  
  # PostgreSQL
  database_type     = "postgresql"
  postgres_host     = module.postgresql.db_instance_address
  postgres_port     = 5432
  postgres_database = "alexander"
  postgres_username = module.postgresql.db_instance_username
  postgres_password = module.postgresql.db_instance_password
  
  # Redis
  enable_redis = true
  redis_host   = module.redis.cluster_address
  redis_port   = 6379
  
  # Storage
  storage_path    = "/data/blobs"
  ebs_volume_size = 500
  ebs_volume_type = "gp3"
  
  # SSL
  enable_ssl          = true
  ssl_certificate_arn = var.certificate_arn
  
  tags = {
    CostCenter = "storage"
    Compliance = "soc2"
  }
}

# Route53 DNS
resource "aws_route53_record" "alexander" {
  zone_id = data.aws_route53_zone.main.zone_id
  name    = var.domain_name
  type    = "A"
  
  alias {
    name                   = module.alexander.alb_dns_name
    zone_id                = module.alexander.alb_zone_id
    evaluate_target_health = true
  }
}

data "aws_route53_zone" "main" {
  name = join(".", slice(split(".", var.domain_name), 1, length(split(".", var.domain_name))))
}

# Outputs
output "endpoint" {
  value = "https://${var.domain_name}"
}

output "alb_endpoint" {
  value = module.alexander.endpoint
}

output "access_key_id" {
  value     = module.alexander.access_key_id
  sensitive = true
}

output "secret_access_key" {
  value     = module.alexander.secret_access_key
  sensitive = true
}

output "rds_endpoint" {
  value = module.postgresql.db_instance_address
}

output "redis_endpoint" {
  value = module.redis.cluster_address
}
