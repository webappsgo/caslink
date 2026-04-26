# API Reference

## Interactive Documentation

- **Swagger UI**: http://localhost:64580/swagger
- **GraphiQL**: http://localhost:64580/graphiql

## REST API

### Create Short URL

```bash
curl -X POST http://localhost:64580/api/v1/urls \
  -H "Content-Type: application/json" \
  -d '{"url": "https://example.com", "custom_code": "ex"}'
```

### Get URL Details

```bash
curl http://localhost:64580/api/v1/urls/ex
```

### Generate QR Code

```bash
curl http://localhost:64580/api/v1/qr/ex > qr.png
```

## See Swagger UI for complete API documentation.
