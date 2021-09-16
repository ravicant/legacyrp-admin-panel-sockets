# LegacyRP Admin Panel Socket Server

This server provides the map socket and map tiles for the admin panel live map.

### Usage

1. Download [.example.env](.example.env) and [admin-panel-sockets.exe](admin-panel-sockets.exe) to a directory of your choice
2. Download the [display-map.json](display-map.json) and [vehicle-map.json](vehicle-map.json) to the same directory
3. Rename the .example.env to .env
4. In the .env, set the SSL_CERT and the SSL_KEY to the path to your SSL certificate and key
5. Add the secret tokens for **every** server (used for accessing the /world.json route)
    - Example for c2s1
    - `c2s1=mytoken`
6. Make sure you open port 8080 to the public
7. Run admin-panel-sockets.exe
