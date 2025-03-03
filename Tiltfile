# Run server dependencies

local_resource(
    'gamesmaster-bot',
    dir='.',
    serve_dir='.',
    cmd='make build',
    serve_cmd='./bin/gamesmaster bot',
    ignore=['./bin', './var', ".git"],
    deps='.',
    labels=['Bots'],
    serve_env={'DEV': 'true'}
)
