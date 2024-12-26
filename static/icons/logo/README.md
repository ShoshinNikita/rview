# Logo

The logo consists of the capital 'R' rendered with Roboto, weight 300.
`svg`s were created with the help of https://github.com/danmarshall/google-font-to-svg-path.

`png`s can be created with the following commands:

```bash
docker run --rm -v $(pwd):/data -it --entrypoint bash node:22-alpine

# https://github.com/domenic/svg2png
npm install svg2png -g

cd /data
svg2png logo.svg --output logo.png --width=512 --height=512
svg2png logo-mask.svg --output logo-mask.png --width=512 --height=512
```
