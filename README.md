# chirpy
A Go web server for a simple micro-blogging platform, based on the Boot.dev **Learn Web Servers** course. Implements API endpoints for user authentication/authorization and chirp (post) management.

# Running the Project

The server expects a `.env` file in its working directory with the following environment variables:

|	Variable  | Description|
|-------------|------------|
| `JWT_SECRET`| Secret key used to sign JWT tokens for user authentication |
| `POLKA_KEY` | API key for the fictional Polka payment processor that interacts with the `api/polka/webhooks` endpoint |

 A helpful script in the repo's root directory will build and run the server in the `/serve` directory with the `--debug` flag enabled.

 ```sh
 bash bar.sh
 ```