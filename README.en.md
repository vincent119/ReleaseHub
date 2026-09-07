# ReleaseHub

ReleaseHub is a release-governance platform that uses Argo CD as its deployment executor. The repository is currently building its platform foundation.

## Repository Structure

- `Server/`: Go API, worker, and migration commands.
- `Web/`: React web application and Nginx runtime.
- `API/`: Source OpenAPI contract.
- `Deployments/`: Helm and Kustomize deployment definitions.
- `Docs/zh-TW/` and `Docs/en/`: Bilingual technical documentation.

## Generate API Code

```bash
make generate
```

## Validate OpenAPI

```bash
make lint-openapi
```

## Documentation

- [Architecture](Docs/en/architecture.md)
- [Configuration](Docs/en/configuration.md)
- [OIDC](Docs/en/oidc.md)
- [Argo CD Integration](Docs/en/argocd.md)
- [Amazon ECR Integration](Docs/en/ecr.md)
- [Deployment](Docs/en/deployment.md)

Run the full repository integration checks with:

```bash
make verify
```

## License

The licensing model has not been selected, so the repository does not currently include a `LICENSE` file.
