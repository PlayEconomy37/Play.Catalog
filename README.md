# Play.Catalog

Play Economy Catalog microservice.

## Setup

- Copy public RSA key generated from Play.Identity

- To convert RSA public key file into an environment variable that can be used by our application, we can use the following code:

```bash
bytes, err := fileSystem.ReadFile("<path to RSA public key>")
if err != nil {
	logger.Fatal(err, nil)
}

s := base64.StdEncoding.EncodeToString(bytes)
print(s)
```

We read in the file and base64 encode it to convert it into a string.

Note: We store this base64 encoded string as a secret in Github to be used in Github Actions.

- Use **dev.json** inside the **config** folder to define the configuration values for our application.
  If there are any sensitive values, we can define them like this:

```bash
export DB__Dsn=mongodb://localhost:27017

go run ./cmd/api
```

Our configuration parser captures environment variables that follow this naming convention:

If we have a nested structure like this:

```bash
{
    "DB": {
        "Dsn": "mongodb://localhost:27017",
        ...
    }
}
```

We can rewrite it as an environment variable like this:

```bash
DB__Dsn
```

Notice the double underscore between each nested key and how the keys must have the same exact case.
