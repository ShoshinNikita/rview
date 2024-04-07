# Screenshots

- [Dirs \& Files](#dirs--files)
- [Previews](#previews)
  - [Image](#image)
  - [Text](#text)
  - [Video](#video)
- [Search](#search)
- [How to update screenshots](#how-to-update-screenshots)

## Dirs & Files

<img src="screenshots/dir_home.jpg"></img>

## Previews

### Image

<img src="screenshots/preview_image.jpg"></img>

### Text

<img src="screenshots/preview_text.jpg"></img>

### Video

<img src="screenshots/preview_video.jpg"></img>

## Search

<img src="screenshots/search.jpg"></img>

## How to update screenshots

1. GitHub doesn't allow adding border to images - so, we have to add it manually. Just
   add the following code to `common.css`:

   ```css
   #app,
   .preview-close-layer {
     border: 1px solid #21262d;
   }
   ```

   Note: `#21262d` is `var(--color-border-muted)` on GitHub.

2. Prepare demo files.
4. Make `.png` screenshots.
5. Use the following command to convert screenshots to `.jpg`:

   ```sh
   vipsthumbnail --size 1600x900 -o ./jpg/%s.jpg[Q=90,optimize_coding] *.png
   ```
