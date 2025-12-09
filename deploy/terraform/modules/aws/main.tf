# Alexander Storage Terraform Module - AWS

terraform {
  required_version = ">= 1.0"
  
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = ">= 5.0"
    }
    random = {
      source  = "hashicorp/random"
      version = ">= 3.0"
    }
  }
}

# Variables
variable "name" {
  description = "Name prefix for resources"
  type        = string
  default     = "alexander"
}

variable "environment" {
  description = "Environment name (dev, staging, production)"
  type        = string
}

variable "region" {
  description = "AWS region"
  type        = string
  default     = "us-east-1"
}

variable "vpc_id" {
  description = "VPC ID for deployment"
  type        = string
}

variable "subnet_ids" {
  description = "Subnet IDs for deployment"
  type        = list(string)
}

variable "instance_type" {
  description = "EC2 instance type"
  type        = string
  default     = "t3.small"
}

variable "allowed_cidr" {
  description = "CIDR block allowed for HTTP/HTTPS access (null means 0.0.0.0/0)"
  type        = string
  default     = null
}

variable "associate_public_ip" {
  description = "Whether to associate public IPs with instances"
  type        = bool
  default     = false
}

variable "min_size" {
  description = "Minimum number of instances"
  type        = number
  default     = 1
}

variable "max_size" {
  description = "Maximum number of instances"
  type        = number
  default     = 5
}

variable "desired_capacity" {
  description = "Desired number of instances"
  type        = number
  default     = 2
}

variable "database_type" {
  description = "Database type (sqlite/postgresql)"
  type        = string
  default     = "sqlite"
}

variable "postgres_host" {
  description = "PostgreSQL host (if database_type is postgresql)"
  type        = string
  default     = ""
}

variable "postgres_port" {
  description = "PostgreSQL port"
  type        = number
  default     = 5432
}

variable "postgres_database" {
  description = "PostgreSQL database name"
  type        = string
  default     = "alexander"
}

variable "postgres_username" {
  description = "PostgreSQL username"
  type        = string
  default     = "alexander"
  sensitive   = true
}

variable "postgres_password" {
  description = "PostgreSQL password"
  type        = string
  default     = ""
  sensitive   = true
}

variable "enable_redis" {
  description = "Enable Redis caching"
  type        = bool
  default     = false
}

variable "redis_host" {
  description = "Redis host"
  type        = string
  default     = ""
}

variable "redis_port" {
  description = "Redis port"
  type        = number
  default     = 6379
}

variable "storage_path" {
  description = "Path for blob storage"
  type        = string
  default     = "/data/blobs"
}

variable "ebs_volume_size" {
  description = "EBS volume size in GB"
  type        = number
  default     = 100
}

variable "ebs_volume_type" {
  description = "EBS volume type"
  type        = string
  default     = "gp3"
}

variable "alexander_version" {
  description = "Alexander Storage version/tag"
  type        = string
  default     = "latest"
}

variable "enable_ssl" {
  description = "Enable SSL/TLS"
  type        = bool
  default     = true
}

variable "ssl_certificate_arn" {
  description = "ACM certificate ARN for SSL"
  type        = string
  default     = ""
}

variable "tags" {
  description = "Additional tags for resources"
  type        = map(string)
  default     = {}
}

variable "enable_alb_access_logs" {
  description = "Enable ALB access logging"
  type        = bool
  default     = true
}

variable "alb_access_logs_bucket" {
  description = "S3 bucket for ALB access logs"
  type        = string
  default     = ""
}

variable "enable_waf" {
  description = "Enable AWS WAF for ALB protection"
  type        = bool
  default     = true
}

variable "waf_web_acl_arn" {
  description = "WAF Web ACL ARN (if not provided and enable_waf is true, a basic ACL will be created)"
  type        = string
  default     = ""
}

# Locals
locals {
  common_tags = merge({
    Name        = var.name
    Environment = var.environment
    ManagedBy   = "terraform"
    Application = "alexander-storage"
  }, var.tags)
}

# Random ID for unique naming
resource "random_id" "this" {
  byte_length = 4
}

# Security Group
resource "aws_security_group" "alexander" {
  name        = "${var.name}-${var.environment}-sg"
  description = "Security group for Alexander Storage"
  vpc_id      = var.vpc_id
  
  tags = local.common_tags
}

# HTTP ingress rule
resource "aws_vpc_security_group_ingress_rule" "http" {
  description       = "Allow HTTP from anywhere"
  security_group_id = aws_security_group.alexander.id
  from_port         = 8080
  to_port           = 8080
  ip_protocol       = "tcp"
  cidr_ipv4         = var.allowed_cidr != null ? var.allowed_cidr : "0.0.0.0/0"
  
  tags = merge(local.common_tags, { Name = "${var.name}-http-ingress" })
}

# HTTPS ingress rule
resource "aws_vpc_security_group_ingress_rule" "https" {
  description       = "Allow HTTPS from anywhere"
  security_group_id = aws_security_group.alexander.id
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
  cidr_ipv4         = var.allowed_cidr != null ? var.allowed_cidr : "0.0.0.0/0"
  
  tags = merge(local.common_tags, { Name = "${var.name}-https-ingress" })
}

# gRPC cluster communication - only from within SG
resource "aws_vpc_security_group_ingress_rule" "grpc" {
  description                  = "Allow gRPC cluster communication within security group"
  security_group_id            = aws_security_group.alexander.id
  from_port                    = 9090
  to_port                      = 9090
  ip_protocol                  = "tcp"
  referenced_security_group_id = aws_security_group.alexander.id
  
  tags = merge(local.common_tags, { Name = "${var.name}-grpc-ingress" })
}

# Egress rule - restrict to known services
resource "aws_vpc_security_group_egress_rule" "allowed_egress" {
  description       = "Allow outbound HTTPS for external services"
  security_group_id = aws_security_group.alexander.id
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
  cidr_ipv4         = "0.0.0.0/0"
  
  tags = merge(local.common_tags, { Name = "${var.name}-https-egress" })
}

resource "aws_vpc_security_group_egress_rule" "dns_egress" {
  description       = "Allow DNS queries"
  security_group_id = aws_security_group.alexander.id
  from_port         = 53
  to_port           = 53
  ip_protocol       = "udp"
  cidr_ipv4         = "0.0.0.0/0"
  
  tags = merge(local.common_tags, { Name = "${var.name}-dns-egress" })
}

# IAM Role for EC2
resource "aws_iam_role" "alexander" {
  name = "${var.name}-${var.environment}-role"
  
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "ec2.amazonaws.com"
        }
      }
    ]
  })
  
  tags = local.common_tags
}

resource "aws_iam_role_policy" "alexander" {
  name = "${var.name}-${var.environment}-policy"
  role = aws_iam_role.alexander.id
  
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "logs:CreateLogGroup",
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Resource = "*"
      },
      {
        Effect = "Allow"
        Action = [
          "ssm:GetParameter",
          "ssm:GetParameters"
        ]
        Resource = "arn:aws:ssm:*:*:parameter/${var.name}/*"
      },
      {
        Effect = "Allow"
        Action = [
          "ec2:DescribeInstances",
          "ec2:DescribeTags"
        ]
        Resource = "*"
      }
    ]
  })
}

resource "aws_iam_instance_profile" "alexander" {
  name = "${var.name}-${var.environment}-profile"
  role = aws_iam_role.alexander.name
}

# Generate admin credentials
resource "random_password" "admin_secret_key" {
  length  = 40
  special = false
}

resource "random_id" "admin_access_key" {
  byte_length = 10
}

# Store credentials in SSM Parameter Store
resource "aws_ssm_parameter" "admin_access_key" {
  name  = "/${var.name}/${var.environment}/admin-access-key"
  type  = "SecureString"
  value = "AK${upper(random_id.admin_access_key.hex)}"
  
  tags = local.common_tags
}

resource "aws_ssm_parameter" "admin_secret_key" {
  name  = "/${var.name}/${var.environment}/admin-secret-key"
  type  = "SecureString"
  value = random_password.admin_secret_key.result
  
  tags = local.common_tags
}

# Launch Template
resource "aws_launch_template" "alexander" {
  name_prefix   = "${var.name}-${var.environment}-"
  image_id      = data.aws_ami.amazon_linux_2.id
  instance_type = var.instance_type
  
  iam_instance_profile {
    name = aws_iam_instance_profile.alexander.name
  }
  
  # SECURITY: Enforce IMDSv2 only
  metadata_options {
    http_endpoint               = "enabled"
    http_tokens                 = "required"  # Enforce IMDSv2
    http_put_response_hop_limit = 1
    instance_metadata_tags      = "enabled"
  }
  
  # SECURITY: Disable public IP assignment by default
  network_interfaces {
    associate_public_ip_address = var.associate_public_ip
    security_groups             = [aws_security_group.alexander.id]
  }
  
  block_device_mappings {
    device_name = "/dev/xvda"
    
    ebs {
      volume_size           = var.ebs_volume_size
      volume_type           = var.ebs_volume_type
      delete_on_termination = false
      encrypted             = true
    }
  }
  
  user_data = base64encode(templatefile("${path.module}/templates/user-data.sh.tpl", {
    name              = var.name
    environment       = var.environment
    region            = var.region
    database_type     = var.database_type
    postgres_host     = var.postgres_host
    postgres_port     = var.postgres_port
    postgres_database = var.postgres_database
    postgres_username = var.postgres_username
    postgres_password = var.postgres_password
    enable_redis      = var.enable_redis
    redis_host        = var.redis_host
    redis_port        = var.redis_port
    storage_path      = var.storage_path
    alexander_version = var.alexander_version
  }))
  
  tag_specifications {
    resource_type = "instance"
    tags          = local.common_tags
  }
  
  tag_specifications {
    resource_type = "volume"
    tags          = local.common_tags
  }
  
  tags = local.common_tags
}

# Auto Scaling Group
resource "aws_autoscaling_group" "alexander" {
  name                = "${var.name}-${var.environment}-asg"
  vpc_zone_identifier = var.subnet_ids
  min_size            = var.min_size
  max_size            = var.max_size
  desired_capacity    = var.desired_capacity
  
  launch_template {
    id      = aws_launch_template.alexander.id
    version = "$Latest"
  }
  
  target_group_arns = [aws_lb_target_group.alexander.arn]
  
  health_check_type         = "ELB"
  health_check_grace_period = 300
  
  instance_refresh {
    strategy = "Rolling"
    preferences {
      min_healthy_percentage = 50
    }
  }
  
  tag {
    key                 = "Name"
    value               = "${var.name}-${var.environment}"
    propagate_at_launch = true
  }
  
  dynamic "tag" {
    for_each = local.common_tags
    content {
      key                 = tag.key
      value               = tag.value
      propagate_at_launch = true
    }
  }
}

# Auto Scaling Policies
resource "aws_autoscaling_policy" "scale_up" {
  name                   = "${var.name}-${var.environment}-scale-up"
  scaling_adjustment     = 1
  adjustment_type        = "ChangeInCapacity"
  cooldown               = 300
  autoscaling_group_name = aws_autoscaling_group.alexander.name
}

resource "aws_autoscaling_policy" "scale_down" {
  name                   = "${var.name}-${var.environment}-scale-down"
  scaling_adjustment     = -1
  adjustment_type        = "ChangeInCapacity"
  cooldown               = 300
  autoscaling_group_name = aws_autoscaling_group.alexander.name
}

# CloudWatch Alarms for Auto Scaling
resource "aws_cloudwatch_metric_alarm" "high_cpu" {
  alarm_name          = "${var.name}-${var.environment}-high-cpu"
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 2
  metric_name         = "CPUUtilization"
  namespace           = "AWS/EC2"
  period              = 300
  statistic           = "Average"
  threshold           = 70
  alarm_description   = "Scale up if CPU > 70%"
  alarm_actions       = [aws_autoscaling_policy.scale_up.arn]
  
  dimensions = {
    AutoScalingGroupName = aws_autoscaling_group.alexander.name
  }
}

resource "aws_cloudwatch_metric_alarm" "low_cpu" {
  alarm_name          = "${var.name}-${var.environment}-low-cpu"
  comparison_operator = "LessThanThreshold"
  evaluation_periods  = 2
  metric_name         = "CPUUtilization"
  namespace           = "AWS/EC2"
  period              = 300
  statistic           = "Average"
  threshold           = 30
  alarm_description   = "Scale down if CPU < 30%"
  alarm_actions       = [aws_autoscaling_policy.scale_down.arn]
  
  dimensions = {
    AutoScalingGroupName = aws_autoscaling_group.alexander.name
  }
}

# Application Load Balancer
resource "aws_lb" "alexander" {
  name               = "${var.name}-${var.environment}-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alexander.id]
  subnets            = var.subnet_ids
  
  enable_deletion_protection = var.environment == "production"
  drop_invalid_header_fields = true
  
  dynamic "access_logs" {
    for_each = var.enable_alb_access_logs && var.alb_access_logs_bucket != "" ? [1] : []
    content {
      bucket  = var.alb_access_logs_bucket
      prefix  = "${var.name}-${var.environment}"
      enabled = true
    }
  }
  
  tags = local.common_tags
}

# WAF Web ACL for ALB protection
resource "aws_wafv2_web_acl" "alexander" {
  count = var.enable_waf && var.waf_web_acl_arn == "" ? 1 : 0
  
  name        = "${var.name}-${var.environment}-waf"
  description = "WAF rules for Alexander Storage ALB"
  scope       = "REGIONAL"
  
  default_action {
    allow {}
  }
  
  rule {
    name     = "AWSManagedRulesCommonRuleSet"
    priority = 1
    
    override_action {
      none {}
    }
    
    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesCommonRuleSet"
        vendor_name = "AWS"
      }
    }
    
    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${var.name}-common-rules"
      sampled_requests_enabled   = true
    }
  }
  
  rule {
    name     = "AWSManagedRulesKnownBadInputsRuleSet"
    priority = 2
    
    override_action {
      none {}
    }
    
    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesKnownBadInputsRuleSet"
        vendor_name = "AWS"
      }
    }
    
    visibility_config {
      cloudwatch_metrics_enabled = true
      metric_name                = "${var.name}-bad-inputs"
      sampled_requests_enabled   = true
    }
  }
  
  visibility_config {
    cloudwatch_metrics_enabled = true
    metric_name                = "${var.name}-waf"
    sampled_requests_enabled   = true
  }
  
  tags = local.common_tags
}

# WAF Association with ALB
resource "aws_wafv2_web_acl_association" "alexander" {
  count = var.enable_waf ? 1 : 0
  
  resource_arn = aws_lb.alexander.arn
  web_acl_arn  = var.waf_web_acl_arn != "" ? var.waf_web_acl_arn : aws_wafv2_web_acl.alexander[0].arn
}

resource "aws_lb_target_group" "alexander" {
  name     = "${var.name}-${var.environment}-tg"
  port     = 8080
  protocol = "HTTP"
  vpc_id   = var.vpc_id
  
  health_check {
    enabled             = true
    healthy_threshold   = 2
    interval            = 30
    matcher             = "200"
    path                = "/health"
    port                = "traffic-port"
    timeout             = 5
    unhealthy_threshold = 3
  }
  
  tags = local.common_tags
}

resource "aws_lb_listener" "http" {
  load_balancer_arn = aws_lb.alexander.arn
  port              = 80
  protocol          = "HTTP"
  
  default_action {
    type = "redirect"
    
    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}

resource "aws_lb_listener" "https" {
  count = var.enable_ssl ? 1 : 0
  
  load_balancer_arn = aws_lb.alexander.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS-1-2-2017-01"
  certificate_arn   = var.ssl_certificate_arn
  
  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.alexander.arn
  }
}

# Data Sources
data "aws_ami" "amazon_linux_2" {
  most_recent = true
  owners      = ["amazon"]
  
  filter {
    name   = "name"
    values = ["amzn2-ami-hvm-*-x86_64-gp2"]
  }
  
  filter {
    name   = "virtualization-type"
    values = ["hvm"]
  }
}

# Outputs
output "endpoint" {
  description = "Alexander Storage endpoint URL"
  value       = var.enable_ssl ? "https://${aws_lb.alexander.dns_name}" : "http://${aws_lb.alexander.dns_name}"
}

output "alb_dns_name" {
  description = "ALB DNS name"
  value       = aws_lb.alexander.dns_name
}

output "alb_zone_id" {
  description = "ALB zone ID for Route53"
  value       = aws_lb.alexander.zone_id
}

output "access_key_id" {
  description = "Admin access key ID"
  value       = aws_ssm_parameter.admin_access_key.value
  sensitive   = true
}

output "secret_access_key" {
  description = "Admin secret access key"
  value       = aws_ssm_parameter.admin_secret_key.value
  sensitive   = true
}

output "security_group_id" {
  description = "Security group ID"
  value       = aws_security_group.alexander.id
}

output "autoscaling_group_name" {
  description = "Auto Scaling group name"
  value       = aws_autoscaling_group.alexander.name
}
