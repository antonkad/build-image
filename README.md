                +--------------------+
                |   Publish Function |
                +--------------------+
                          |
                          v
                +--------------------+
                | Determine Framework|
                +--------------------+
                          |
        +-----------------+-----------------+
        |                                   |
        v                                   v
+------------------+              +------------------+
| Framework: Next  |              | Framework: Others|
+------------------+              +------------------+
        |                                   |
        v                                   v
+------------------------+        +-----------------------+
| Run buildNext helper   |        | Run buildNginx helper |
| - Set entrypoint to    |        | - Serve via Nginx     |
|   start app            |        |   on port 80          |
+------------------------+        +-----------------------+
        |                                   |
        v                                   v
+------------------+              +------------------+
|    Run build     |              |     Run build    |
+------------------+              +------------------+

# Testing Build function
```
dagger call -j build --repository=https://github.com/user/repo --ref=main --path=examples/vue/
dagger call -j build --repository=https://github.com/user/repo --ref=main --path=examples/angular/
dagger call -j build --repository=https://github.com/user/repo --ref=main --path=examples/create-react-app/
dagger call -j build --repository=https://github.com/user/repo --ref=main --path=examples/svelte/
dagger call -j build --repository=https://github.com/user/repo --ref=main --path=examples/nextjs/
```
# Testing Publish function (calls BuildNext or BuildNginx internally)
```
dagger call -j publish --repository=https://github.com/user/repo --ref=main --path=examples/vue/ --framework=vue 
dagger call -j publish --repository=https://github.com/user/repo --ref=main --path=examples/angular/ --framework=angular 
dagger call -j publish --repository=https://github.com/user/repo --ref=main --path=examples/create-react-app/ --framework=react
dagger call -j publish --repository=https://github.com/user/repo --ref=main --path=examples/svelte/ --framework=svelte 
dagger call -j publish --repository=https://github.com/user/repo --ref=main --path=examples/nextjs/ --framework=next 
```

# Testing build-nginx function (calls Build internally)
```
dagger call -j build-nginx --repository=https://github.com/user/repo --ref=main --path=examples/vue/ --framework=vue
dagger call -j build-nginx --repository=https://github.com/user/repo --ref=main --path=examples/angular/ --framework=angular 
dagger call -j build-nginx --repository=https://github.com/user/repo --ref=main --path=examples/create-react-app/ --framework=react
dagger call -j build-nginx --repository=https://github.com/user/repo --ref=main --path=examples/svelte/ --framework=svelte 
```
# Testing build-next function (calls Build internally)
```
dagger call -j build-next --repository=https://github.com/user/repo --ref=main --path=examples/nextjs/ --framework=next 

```