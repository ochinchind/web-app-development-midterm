. + {
  info: { title: "Cargo Transport Service API", version: "1.0" },
  host: "localhost:8080",
  schemes: ["http"],
  securityDefinitions: {
    bearer: {
      type: "apiKey",
      name: "Authorization",
      in: "header",
      description: "Format: Bearer <access_token>"
    }
  },
  security: [{ bearer: [] }]
}
