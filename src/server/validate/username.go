package validate

import (
	"fmt"
	"regexp"
	"strings"
)

// Username validation rules per AI.md PART 23
var (
	usernameRegex = regexp.MustCompile(`^[a-z][a-z0-9_-]*[a-z0-9]$`)
	consecutiveRegex = regexp.MustCompile(`__|--|_-|-_`)
)

// UsernameBlocklist per AI.md PART 23
var UsernameBlocklist = []string{
	// System & Administrative
	"admin", "administrator", "root", "system", "sysadmin", "superuser",
	"master", "owner", "operator", "manager", "moderator", "mod",
	"staff", "support", "helpdesk", "help", "service", "daemon",

	// Server & Technical
	"server", "host", "node", "cluster", "api", "www", "web", "mail",
	"email", "smtp", "ftp", "ssh", "dns", "proxy", "gateway", "router",
	"firewall", "localhost", "local", "internal", "external", "public",
	"private", "network", "database", "db", "cache", "redis", "mysql",
	"postgres", "mongodb", "elastic", "nginx", "apache", "docker",

	// Application & Service Names
	"app", "application", "bot", "robot", "crawler", "spider", "scraper",
	"webhook", "callback", "cron", "scheduler", "worker", "queue", "job",
	"task", "process", "service", "microservice", "lambda", "function",

	// Authentication & Security
	"auth", "authentication", "login", "logout", "signin", "signout",
	"signup", "register", "password", "passwd", "token", "oauth", "sso",
	"saml", "ldap", "kerberos", "security", "secure", "ssl", "tls",
	"certificate", "cert", "key", "secret", "credential", "session",

	// Roles & Permissions
	"guest", "anonymous", "anon", "user", "users", "member", "members",
	"subscriber", "editor", "author", "contributor", "reviewer", "auditor",
	"analyst", "developer", "dev", "devops", "engineer", "architect",
	"designer", "tester", "qa", "billing", "finance", "legal", "hr",
	"sales", "marketing", "ceo", "cto", "cfo", "coo", "founder", "cofounder",

	// Common Reserved
	"account", "accounts", "profile", "profiles", "settings", "config",
	"configuration", "dashboard", "panel", "console", "portal", "home",
	"index", "main", "default", "null", "nil", "undefined", "void",
	"true", "false", "test", "testing", "debug", "demo", "example",
	"sample", "temp", "temporary", "tmp", "backup", "archive", "log",
	"logs", "audit", "report", "reports", "analytics", "stats", "status",

	// API & Endpoints
	"api", "rest", "graphql", "grpc", "websocket", "ws", "wss", "http",
	"https", "endpoint", "endpoints", "route", "routes", "path", "url",
	"uri", "callback", "hook", "hooks", "event", "events", "stream",

	// Content & Media
	"blog", "news", "article", "articles", "post", "posts", "page", "pages",
	"feed", "rss", "atom", "sitemap", "robots", "favicon", "static",
	"assets", "images", "image", "img", "media", "upload", "uploads",
	"download", "downloads", "file", "files", "document", "documents",

	// Communication
	"contact", "message", "messages", "chat", "notification", "notifications",
	"alert", "alerts", "inbox", "outbox", "sent", "draft", "drafts",
	"spam", "abuse", "report", "flag", "block", "mute", "ban",

	// Commerce & Billing
	"shop", "store", "cart", "checkout", "order", "orders", "invoice",
	"invoices", "payment", "payments", "subscription", "subscriptions",
	"plan", "plans", "pricing", "billing", "refund", "coupon", "discount",

	// Social Features
	"follow", "follower", "followers", "following", "friend", "friends",
	"like", "likes", "share", "shares", "comment", "comments", "reply",
	"mention", "mentions", "tag", "tags", "group", "groups", "team", "teams",
	"community", "communities", "forum", "forums", "channel", "channels",

	// Brand & Legal
	"official", "verified", "trusted", "partner", "affiliate", "sponsor",
	"brand", "trademark", "copyright", "legal", "terms", "privacy",
	"policy", "policies", "tos", "eula", "gdpr", "dmca", "abuse",

	// Offensive / Impersonation Prevention
	"fuck", "shit", "ass", "bitch", "bastard", "damn", "cunt", "dick",
	"penis", "vagina", "sex", "porn", "xxx", "nude", "naked", "nsfw",
	"kill", "murder", "death", "die", "suicide", "hate", "nazi", "hitler",
	"racist", "racism", "terrorist", "terrorism", "isis", "alqaeda",

	// Numbers & Special
	"0", "1", "123", "1234", "12345", "000", "111", "666", "911", "420", "69",

	// Common Spam Patterns
	"info", "noreply", "no-reply", "donotreply", "mailer", "postmaster",
	"webmaster", "hostmaster", "abuse", "spam", "junk", "trash",

	// Project-specific
	"caslink", "casapps",
}

// Critical terms that also block as substrings
var criticalTerms = []string{
	"admin", "root", "system", "mod", "official", "verified",
}

// ValidateUsername validates a username per AI.md PART 23 rules
func ValidateUsername(username string, isAdmin bool) error {
	// Server admins are exempt from blocklist
	if isAdmin {
		return nil
	}

	// Convert to lowercase (case-insensitive)
	username = strings.ToLower(strings.TrimSpace(username))

	// Check length
	if len(username) < 3 {
		return fmt.Errorf("username must be at least 3 characters")
	}
	if len(username) > 32 {
		return fmt.Errorf("username cannot exceed 32 characters")
	}

	// Check format (must start with letter, lowercase alphanumeric + _ -)
	if !usernameRegex.MatchString(username) {
		return fmt.Errorf("username can only contain lowercase letters, numbers, underscore, and hyphen")
	}

	// Check for consecutive special chars
	if consecutiveRegex.MatchString(username) {
		return fmt.Errorf("username cannot contain consecutive underscores or hyphens")
	}

	// Check blocklist (exact match, case-insensitive)
	for _, blocked := range UsernameBlocklist {
		if username == strings.ToLower(blocked) {
			return fmt.Errorf("username contains blocked word: %s", blocked)
		}
	}

	// Check critical terms as substrings
	for _, critical := range criticalTerms {
		if strings.Contains(username, strings.ToLower(critical)) {
			return fmt.Errorf("username contains blocked word: %s", critical)
		}
	}

	return nil
}

// ValidateEmail validates an email address
func ValidateEmail(email string) error {
	email = strings.ToLower(strings.TrimSpace(email))

	if len(email) == 0 {
		return fmt.Errorf("email is required")
	}

	// Basic email regex (simple validation)
	emailRegex := regexp.MustCompile(`^[a-z0-9._%+-]+@[a-z0-9.-]+\.[a-z]{2,}$`)
	if !emailRegex.MatchString(email) {
		return fmt.Errorf("please enter a valid email address")
	}

	return nil
}
