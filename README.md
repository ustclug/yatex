# yatex

Yet Another Thumbnail indEX

## Service

Accepts GET requests in the format `/path/to/image@{dimension}@{quality}.{ext}` and generates downscaled images that satisfies the request, then returns `X-Accel-Redirect` so Nginx picks the file up instead of streaming from this service.
The file is always saved to disk so future requests to the same path can be handled by the frontend Nginx directly.
If the soruce file already satisfies the dimensions, a symlink is generated instead.

Both the `dimension` and `quality` parts must be whitelisted in config file, or this service refuses to generate them with HTTP 404.

Additionally, PNG files do not accept the `quality` argument.
