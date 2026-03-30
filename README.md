# build-image

Dagger module for building and publishing container images from git repositories.

```
            +--------------------+
            |   Publish Function |
            +--------------------+
                      |
                      v
            +--------------------+
            | Determine Framework|
            +--------------------+
                      |
        +-------------+-------------+
        |                           |
        v                           v
+------------------+      +------------------+
| Node server      |      | Static (Nginx)   |
| (next, nuxt,     |      | (react, vue,     |
|  remix, sveltekit)|     |  svelte, angular) |
+------------------+      +------------------+
        |                           |
        v                           v
+------------------+      +------------------+
| Build & publish  |      | Build & publish  |
+------------------+      +------------------+
```

## Usage

```bash
dagger call -m github.com/antonkad/build-image publish \
  --repository=https://github.com/user/repo \
  --ref=main \
  --framework=react \
  --image-name=my-app \
  --commit-hash=abc123 \
  --registry-url=registry.example.com \
  --registry-user=admin \
  --registry-password=env:REGISTRY_PASSWORD
```

### Optional flags

```
--path              Subdirectory within the repo
--package-manager   Override package manager (default: pnpm for node)
--dependencies-cmd  Override install command
--build-cmd         Override build command
--exposed-port      Override default port
--job               Job ID for telemetry
--jobAttempt        Job attempt ID for telemetry
```

### Supported frameworks

| Framework   | Builder        |
|-------------|----------------|
| react       | static-nginx   |
| vue         | static-nginx   |
| svelte      | static-nginx   |
| angular     | static-nginx   |
| vite        | static-nginx   |
| astro       | static-nginx   |
| gatsby      | static-nginx   |
| nextjs      | node-server    |
| nuxt        | node-server    |
| remix       | node-server    |
| sveltekit   | node-server    |
| go          | go-binary      |
| fastapi     | python-server  |
| flask       | python-server  |
| django      | python-server  |
| spring-boot | java-maven     |
| rust        | rust-binary    |
| dockerfile  | custom         |
