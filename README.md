# LegacyRP Admin Panel Socket Server

This server provides the map socket and map tiles for the admin panel live map.

### Usage

1. Download [.example.env](.example.env), [admin-panel-sockets.exe](admin-panel-sockets.exe) and [tiles.tar.gz](tiles.tar.gz) to a directory of your choice
    - You dont need to untar the tiles.tar.gz, the application will do that for you
2. Rename the .example.env to .env
3. In the .env, set the SSL_CERT and the SSL_KEY to the path to your SSL certificate and key
4. Add the secret tokens for **every** server (used for accessing the /world.json route)
    - Example for c2s1
    - `c2s1=mytoken`
5. Make sure you open port 8080 to the public
6. Run admin-panel-sockets.exe