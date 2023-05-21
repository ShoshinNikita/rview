package rview

type FileType string

const (
	FileTypeUnknown FileType = ""
	FileTypeImage   FileType = "image"
	FileTypeAudio   FileType = "audio"
	FileTypeVideo   FileType = "video"
	FileTypeText    FileType = "text"
)

func GetFileType(id FileID) FileType {
	return fileTypesByExtension[id.GetExt()]
}

var fileTypesByExtension = map[string]FileType{
	// Image
	".bmp":  FileTypeImage,
	".gif":  FileTypeImage,
	".ico":  FileTypeImage,
	".jpg":  FileTypeImage,
	".png":  FileTypeImage,
	".jpeg": FileTypeImage,
	".webp": FileTypeImage,
	".heic": FileTypeImage,

	// Audio
	".flac": FileTypeAudio,
	".aif":  FileTypeAudio,
	".mpa":  FileTypeAudio,
	".wma":  FileTypeAudio,
	".wpl":  FileTypeAudio,
	".ogg":  FileTypeAudio,
	".wav":  FileTypeAudio,
	".mp3":  FileTypeAudio,

	// Video
	".avi":  FileTypeVideo,
	".mkv":  FileTypeVideo,
	".mov":  FileTypeVideo,
	".mpg":  FileTypeVideo,
	".mpeg": FileTypeVideo,
	".mp4":  FileTypeVideo,
	".webm": FileTypeVideo,

	// Text (from https://github.com/github/linguist/blob/master/lib/linguist/languages.yml)
	".cfg":           FileTypeText,
	".m":             FileTypeText,
	".pic":           FileTypeText,
	".pb":            FileTypeText,
	".mm":            FileTypeText,
	".asn":           FileTypeText,
	".d":             FileTypeText,
	".self":          FileTypeText,
	".marko":         FileTypeText,
	".lean":          FileTypeText,
	".nl":            FileTypeText,
	".conllu":        FileTypeText,
	".reb":           FileTypeText,
	".robot":         FileTypeText,
	".xpm":           FileTypeText,
	".properties":    FileTypeText,
	".pas":           FileTypeText,
	".brs":           FileTypeText,
	".owl":           FileTypeText,
	".q":             FileTypeText,
	".cshtml":        FileTypeText,
	".e":             FileTypeText,
	".befunge":       FileTypeText,
	".raml":          FileTypeText,
	".opa":           FileTypeText,
	".ex":            FileTypeText,
	".ur":            FileTypeText,
	".mq4":           FileTypeText,
	".mq5":           FileTypeText,
	".agda":          FileTypeText,
	".thrift":        FileTypeText,
	".xm":            FileTypeText,
	".srt":           FileTypeText,
	".jinja":         FileTypeText,
	".sass":          FileTypeText,
	".pbt":           FileTypeText,
	".edn":           FileTypeText,
	".sublime-build": FileTypeText,
	".blade":         FileTypeText,
	".pp":            FileTypeText,
	".cr":            FileTypeText,
	".bison":         FileTypeText,
	".druby":         FileTypeText,
	".gn":            FileTypeText,
	".ebuild":        FileTypeText,
	".j":             FileTypeText,
	".bdf":           FileTypeText,
	".ftl":           FileTypeText,
	".f90":           FileTypeText,
	".swift":         FileTypeText,
	".phtml":         FileTypeText,
	".ipynb":         FileTypeText,
	".tpl":           FileTypeText,
	".cp":            FileTypeText,
	".cob":           FileTypeText,
	".go":            FileTypeText,
	".ring":          FileTypeText,
	".php":           FileTypeText,
	".csd":           FileTypeText,
	".bsl":           FileTypeText,
	".i3":            FileTypeText,
	".http":          FileTypeText,
	".ms":            FileTypeText,
	".raw":           FileTypeText,
	".mak":           FileTypeText,
	".bb":            FileTypeText,
	".ml":            FileTypeText,
	".textile":       FileTypeText,
	".m4":            FileTypeText,
	".sed":           FileTypeText,
	".v":             FileTypeText,
	".do":            FileTypeText,
	".gitignore":     FileTypeText,
	".red":           FileTypeText,
	".ahk":           FileTypeText,
	".yar":           FileTypeText,
	".d-objdump":     FileTypeText,
	".xpl":           FileTypeText,
	".xsp-config":    FileTypeText,
	".volt":          FileTypeText,
	".as":            FileTypeText,
	".lsl":           FileTypeText,
	".jade":          FileTypeText,
	".rs":            FileTypeText,
	".minid":         FileTypeText,
	".t":             FileTypeText,
	".coffee":        FileTypeText,
	".html":          FileTypeText,
	".awk":           FileTypeText,
	".dot":           FileTypeText,
	".org":           FileTypeText,
	".idr":           FileTypeText,
	".pcss":          FileTypeText,
	".pgsql":         FileTypeText,
	".agc":           FileTypeText,
	".mod":           FileTypeText,
	".c":             FileTypeText,
	".myt":           FileTypeText,
	".hlsl":          FileTypeText,
	".p4":            FileTypeText,
	".thy":           FileTypeText,
	".eex":           FileTypeText,
	".scaml":         FileTypeText,
	".clw":           FileTypeText,
	".sh-session":    FileTypeText,
	".pod":           FileTypeText,
	".graphql":       FileTypeText,
	".lhs":           FileTypeText,
	".erb":           FileTypeText,
	".au3":           FileTypeText,
	".gs":            FileTypeText,
	".moo":           FileTypeText,
	".hcl":           FileTypeText,
	".chs":           FileTypeText,
	".jsp":           FileTypeText,
	".sp":            FileTypeText,
	".cmake":         FileTypeText,
	".clj":           FileTypeText,
	".shen":          FileTypeText,
	".wdl":           FileTypeText,
	".hxml":          FileTypeText,
	".yml":           FileTypeText,
	".tcl":           FileTypeText,
	".nf":            FileTypeText,
	".ini":           FileTypeText,
	".xtend":         FileTypeText,
	".pytb":          FileTypeText,
	".epj":           FileTypeText,
	".glsl":          FileTypeText,
	".uc":            FileTypeText,
	".eb":            FileTypeText,
	".jq":            FileTypeText,
	".vhdl":          FileTypeText,
	".pyx":           FileTypeText,
	".sv":            FileTypeText,
	".objdump":       FileTypeText,
	".smt2":          FileTypeText,
	".regexp":        FileTypeText,
	".gitconfig":     FileTypeText,
	".json5":         FileTypeText,
	".yasnippet":     FileTypeText,
	".anim":          FileTypeText,
	".b":             FileTypeText,
	".rexx":          FileTypeText,
	".lfe":           FileTypeText,
	".oxygene":       FileTypeText,
	".asm":           FileTypeText,
	".flex":          FileTypeText,
	".cs":            FileTypeText,
	".pde":           FileTypeText,
	".fx":            FileTypeText,
	".webidl":        FileTypeText,
	".json":          FileTypeText,
	".asc":           FileTypeText,
	".matlab":        FileTypeText,
	".ls":            FileTypeText,
	".stan":          FileTypeText,
	".mtml":          FileTypeText,
	".pl":            FileTypeText,
	".jsx":           FileTypeText,
	".proto":         FileTypeText,
	".ch":            FileTypeText,
	".xml":           FileTypeText,
	".svg":           FileTypeText,
	".mathematica":   FileTypeText,
	".lol":           FileTypeText,
	".ooc":           FileTypeText,
	".xslt":          FileTypeText,
	".rmd":           FileTypeText,
	".afm":           FileTypeText,
	".mask":          FileTypeText,
	".sch":           FileTypeText,
	".em":            FileTypeText,
	".lvproj":        FileTypeText,
	".n":             FileTypeText,
	".cu":            FileTypeText,
	".krl":           FileTypeText,
	".vim":           FileTypeText,
	".pony":          FileTypeText,
	".sci":           FileTypeText,
	".1":             FileTypeText,
	".rpy":           FileTypeText,
	".sparql":        FileTypeText,
	".applescript":   FileTypeText,
	".txt":           FileTypeText,
	".sage":          FileTypeText,
	".ck":            FileTypeText,
	".g4":            FileTypeText,
	".fs":            FileTypeText,
	".fy":            FileTypeText,
	".fst":           FileTypeText,
	".pir":           FileTypeText,
	".st":            FileTypeText,
	".ice":           FileTypeText,
	".monkey":        FileTypeText,
	".pogo":          FileTypeText,
	".el":            FileTypeText,
	".js":            FileTypeText,
	".pro":           FileTypeText,
	".abap":          FileTypeText,
	".pasm":          FileTypeText,
	".cw":            FileTypeText,
	".sl":            FileTypeText,
	".l":             FileTypeText,
	".spec":          FileTypeText,
	".erl":           FileTypeText,
	".mms":           FileTypeText,
	".dae":           FileTypeText,
	".scm":           FileTypeText,
	".nut":           FileTypeText,
	".py":            FileTypeText,
	".nanorc":        FileTypeText,
	".latte":         FileTypeText,
	".ne":            FileTypeText,
	".iss":           FileTypeText,
	".ebnf":          FileTypeText,
	".ipf":           FileTypeText,
	".chpl":          FileTypeText,
	".coq":           FileTypeText,
	".dylan":         FileTypeText,
	".lagda":         FileTypeText,
	".gradle":        FileTypeText,
	".clp":           FileTypeText,
	".axs.erb":       FileTypeText,
	".eclass":        FileTypeText,
	".xbm":           FileTypeText,
	".als":           FileTypeText,
	".groovy":        FileTypeText,
	".w":             FileTypeText,
	".ol":            FileTypeText,
	".pls":           FileTypeText,
	".purs":          FileTypeText,
	".jl":            FileTypeText,
	".bf":            FileTypeText,
	".hs":            FileTypeText,
	".ncl":           FileTypeText,
	".vb":            FileTypeText,
	".io":            FileTypeText,
	".rg":            FileTypeText,
	".haml":          FileTypeText,
	".djs":           FileTypeText,
	".ps1":           FileTypeText,
	".ts":            FileTypeText,
	".dart":          FileTypeText,
	".edc":           FileTypeText,
	".vcl":           FileTypeText,
	".zig":           FileTypeText,
	".ceylon":        FileTypeText,
	".fr":            FileTypeText,
	".g":             FileTypeText,
	".aj":            FileTypeText,
	".sh":            FileTypeText,
	".orc":           FileTypeText,
	".tcsh":          FileTypeText,
	".prg":           FileTypeText,
	".elm":           FileTypeText,
	".jison":         FileTypeText,
	".x":             FileTypeText,
	".desktop":       FileTypeText,
	".sc":            FileTypeText,
	".nginxconf":     FileTypeText,
	".re":            FileTypeText,
	".yang":          FileTypeText,
	".com":           FileTypeText,
	".sas":           FileTypeText,
	".ninja":         FileTypeText,
	".grace":         FileTypeText,
	".cl":            FileTypeText,
	".creole":        FileTypeText,
	".kt":            FileTypeText,
	".opal":          FileTypeText,
	".8xp":           FileTypeText,
	".ML":            FileTypeText,
	".cfc":           FileTypeText,
	".bat":           FileTypeText,
	".oz":            FileTypeText,
	".ox":            FileTypeText,
	".gsp":           FileTypeText,
	".roff":          FileTypeText,
	".rl":            FileTypeText,
	".handlebars":    FileTypeText,
	".less":          FileTypeText,
	".zone":          FileTypeText,
	".pd":            FileTypeText,
	".ecr":           FileTypeText,
	".kicad_pcb":     FileTypeText,
	".ld":            FileTypeText,
	".f":             FileTypeText,
	".apl":           FileTypeText,
	".hh":            FileTypeText,
	".toc":           FileTypeText,
	".numpy":         FileTypeText,
	".sqf":           FileTypeText,
	".glf":           FileTypeText,
	".fea":           FileTypeText,
	".cy":            FileTypeText,
	".java":          FileTypeText,
	".scala":         FileTypeText,
	".scad":          FileTypeText,
	".apacheconf":    FileTypeText,
	".asy":           FileTypeText,
	".mediawiki":     FileTypeText,
	".vue":           FileTypeText,
	".gd":            FileTypeText,
	".gbr":           FileTypeText,
	".capnp":         FileTypeText,
	".factor":        FileTypeText,
	".reg":           FileTypeText,
	".darcspatch":    FileTypeText,
	".fth":           FileTypeText,
	".hy":            FileTypeText,
	".ec":            FileTypeText,
	".scss":          FileTypeText,
	".cls":           FileTypeText,
	".rb":            FileTypeText,
	".ly":            FileTypeText,
	".zimpl":         FileTypeText,
	".kid":           FileTypeText,
	".golo":          FileTypeText,
	".cson":          FileTypeText,
	".sql":           FileTypeText,
	".metal":         FileTypeText,
	".gml":           FileTypeText,
	".md":            FileTypeText,
	".ni":            FileTypeText,
	".lgt":           FileTypeText,
	".mo":            FileTypeText,
	".boo":           FileTypeText,
	".csv":           FileTypeText,
	".eq":            FileTypeText,
	".mtl":           FileTypeText,
	".css":           FileTypeText,
	".uno":           FileTypeText,
	".ttl":           FileTypeText,
	".c-objdump":     FileTypeText,
	".rdoc":          FileTypeText,
	".abnf":          FileTypeText,
	".ampl":          FileTypeText,
	".cfm":           FileTypeText,
	".cirru":         FileTypeText,
	".rst":           FileTypeText,
	".hb":            FileTypeText,
	".y":             FileTypeText,
	".xojo_code":     FileTypeText,
	".bmx":           FileTypeText,
	".pig":           FileTypeText,
	".tl":            FileTypeText,
	".lasso":         FileTypeText,
	".mako":          FileTypeText,
	".gms":           FileTypeText,
	".icl":           FileTypeText,
	".arc":           FileTypeText,
	".wast":          FileTypeText,
	".spin":          FileTypeText,
	".po":            FileTypeText,
	".rsc":           FileTypeText,
	".x10":           FileTypeText,
	".ston":          FileTypeText,
	".muf":           FileTypeText,
	".dats":          FileTypeText,
	".adb":           FileTypeText,
	".nc":            FileTypeText,
	".rhtml":         FileTypeText,
	".nu":            FileTypeText,
	".flf":           FileTypeText,
	".asp":           FileTypeText,
	".nsi":           FileTypeText,
	".vala":          FileTypeText,
	".ecl":           FileTypeText,
	".bsv":           FileTypeText,
	".axs":           FileTypeText,
	".6pl":           FileTypeText,
	".qml":           FileTypeText,
	".eml":           FileTypeText,
	".sls":           FileTypeText,
	".brd":           FileTypeText,
	".fish":          FileTypeText,
	".fan":           FileTypeText,
	".pike":          FileTypeText,
	".s":             FileTypeText,
	".xc":            FileTypeText,
	".ijs":           FileTypeText,
	".asciidoc":      FileTypeText,
	".for":           FileTypeText,
	".tex":           FileTypeText,
	".pep":           FileTypeText,
	".tla":           FileTypeText,
	".r":             FileTypeText,
	".lua":           FileTypeText,
	".xs":            FileTypeText,
	".smali":         FileTypeText,
	".bal":           FileTypeText,
	".upc":           FileTypeText,
	".ps":            FileTypeText,
	".tea":           FileTypeText,
	".feature":       FileTypeText,
	".styl":          FileTypeText,
	".wisp":          FileTypeText,
	".gdb":           FileTypeText,
	".apib":          FileTypeText,
	".diff":          FileTypeText,
	".cppobjdump":    FileTypeText,
	".twig":          FileTypeText,
	".zep":           FileTypeText,
	".click":         FileTypeText,
	".obj":           FileTypeText,
	".dm":            FileTypeText,
	".ik":            FileTypeText,
	".gp":            FileTypeText,
	".jsonld":        FileTypeText,
	".dwl":           FileTypeText,
	".p":             FileTypeText,
	".hx":            FileTypeText,
	".sfd":           FileTypeText,
	".mu":            FileTypeText,
	".soy":           FileTypeText,
	".pan":           FileTypeText,
	".lookml":        FileTypeText,
	".txl":           FileTypeText,
	".liquid":        FileTypeText,
	".nim":           FileTypeText,
	".dockerfile":    FileTypeText,
	".maxpat":        FileTypeText,
	".lisp":          FileTypeText,
	".kit":           FileTypeText,
	".nix":           FileTypeText,
	".sss":           FileTypeText,
	".toml":          FileTypeText,
	".xquery":        FileTypeText,
	".nit":           FileTypeText,
	".pov":           FileTypeText,
	".ll":            FileTypeText,
	".E":             FileTypeText,
	".parrot":        FileTypeText,
	".gf":            FileTypeText,
	".mumps":         FileTypeText,
	".psc":           FileTypeText,
	".cpp":           FileTypeText,
	".rnh":           FileTypeText,
	".mss":           FileTypeText,
	".cwl":           FileTypeText,
	".shader":        FileTypeText,
	".pkl":           FileTypeText,
	".sco":           FileTypeText,
	".rbbas":         FileTypeText,
	".ejs":           FileTypeText,
	".moon":          FileTypeText,
	".pwn":           FileTypeText,
	".jisonlex":      FileTypeText,
	".aug":           FileTypeText,
	".slim":          FileTypeText,
	".irclog":        FileTypeText,
	".bro":           FileTypeText,
	".omgrofl":       FileTypeText,
	".rkt":           FileTypeText,
	".nlogo":         FileTypeText,
	".litcoffee":     FileTypeText,
}
