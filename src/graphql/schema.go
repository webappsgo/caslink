package graphql

// GetSchema returns the GraphQL schema
func GetSchema() string {
	return `
type Query {
  # Health check
  health: Health!

  # Version information
  version: Version!

  # Get URL by short code
  url(code: String!): URL

  # List URLs (with pagination)
  urls(limit: Int = 10, offset: Int = 0): [URL!]!
}

type Mutation {
  # Create a new short URL
  createURL(input: CreateURLInput!): URL!

  # Update an existing URL
  updateURL(code: String!, input: UpdateURLInput!): URL!

  # Delete a URL
  deleteURL(code: String!): Boolean!
}

type Health {
  status: String!
  version: String!
  mode: String!
  uptime: String!
  timestamp: String!
}

type Version {
  version: String!
  commit: String!
  built: String!
  go_version: String!
  os: String!
  arch: String!
}

type URL {
  id: Int!
  short_code: String!
  long_url: String!
  title: String
  description: String
  custom_code: Boolean!
  expires_at: String
  created_at: String!
  updated_at: String!
}

input CreateURLInput {
  url: String!
  custom_code: String
  title: String
  description: String
  password: String
  expire_after: String
}

input UpdateURLInput {
  url: String
  title: String
  description: String
  password: String
}
`
}
