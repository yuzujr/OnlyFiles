## Quick start

```bash
# 1) Start the server
go run main.go

# 2) Build the code generator
go build -o gen code-gen/code-gen.go

# 3) Generate a one-time code (prints and writes CODE.code)
./gen
```

## Browse & download

1. Open `http://<server>:8080`.
2. Click folders to enter; click a file name to **download**.

## Upload with one-time code

1. Send the one-time code to your friend.
2. They open the site, enter the code, choose/drag a file, click **Upload**.

## UI
<img width="1164" height="661" alt="onlyFiles" src="https://github.com/user-attachments/assets/426edb61-c681-48cf-8712-d0a21d261d91" />
Access my website to preview -> [www.sheot.cn](https://www.sheot.cn/downloads/)

## Notes

* Files live under `./files/` (change `ROOT_DIR` in `main.go` if needed).
* If you are using Nginx, change `PREFIX` in `static/app.js` if needed.
