# Data Mining

Esta pasta separa a fonte do APK, artefatos intermediarios e dados derivados.

## Layout

- `*.apkm`: pacotes fonte, preservados com hash SHA-256.
- `source/`: APKs e splits extraidos do APKM.
- `unity-input/`: arvore Unity consolidada para UnityPy/AssetRipper.
- `il2cpp/`: dump de classes, metadata e DLLs dummy do IL2CPP.
- `tools/`: scripts reproduziveis de extracao estatica e runtime.
- `derived/`: JSONs, relatorios e imagens extraidos.

Os diretorios grandes de entrada e os resultados gerados nao devem ser editados manualmente.

## Extracao estatica

```powershell
python analysis/tools/extract_static.py `
  --input analysis/unity-input `
  --il2cpp analysis/il2cpp `
  --output analysis/derived/6.1.1 `
  --extra-names analysis/derived/6.1.1/normalized/items.json
```

Saidas principais:

- `manifest.json`: origem, hash, contagens e ferramentas.
- `items/assets.json`: sprites, prefab, celulas e posicoes de estrela disponiveis no APK.
- `items/localization.json`: tabelas de nome, habilidade e niveis por idioma.
- `schema.json`: campos relevantes identificados no dump IL2CPP.

`--extra-names` inclui sprites cujo nome nao esta na tabela de localizacao, mas foi
encontrado na captura runtime. A extracao expandida atual encontrou 991 sprites.

## Captura runtime

As definicoes de item nao ficam como `ScriptableObject` no APK base desta versao. O
cliente as materializa durante a inicializacao, por isso stats, crescimento e receitas
sao coletados com Frida:

```powershell
python analysis/tools/collect_runtime.py `
  --package com.rapidfiregames.backpackbrawl `
  --output analysis/derived/6.1.1/runtime
```

O coletor usa o app ja instalado no aparelho via ADB/Frida, nao instala APK nem envia
dados para fora. Se a carga de conteudo ainda nao estiver pronta, ele aguarda e tenta
novamente; o resultado informa a quantidade capturada e os erros por objeto.

Nota operacional: nesta sessao o attach Frida fez a variante mod reiniciar em algumas
janelas. O servidor foi encerrado depois da captura; nao execute novas capturas sem
aceitar esse comportamento e preservar o estado do aparelho.

Para juntar a captura com assets e localizacao:

```powershell
python analysis/tools/normalize_runtime.py `
  --static analysis/derived/6.1.1 `
  --runtime analysis/derived/6.1.1/runtime-final/runtime.json `
  --output analysis/derived/6.1.1/normalized
```

## Modelo de dados

O coletor preserva os conceitos do cliente: `asset_id`, `base_shape`, `star_shape`,
`stats`, `levels`, `star_condition` e `recipes`. A conversao para o catalogo do solver
e feita por `build_catalog.py`. Para todos os itens, `types`, `shape` e
`star_positions` do runtime substituem os valores curados. As regras de alvo e efeito
das estrelas sao mantidas como complemento curado porque `star_status` nao foi
resolvido na captura; novas posicoes sem regra ficam como `rule_status=unknown`.
Stats, niveis e outros campos runtime permanecem em `data/item-metadata.json`.

## Geracao do catalogo

Partindo do catalogo curado versionado em `data/catalog-curated.json`:

```powershell
python analysis/tools/build_catalog.py `
  --current data/catalog-curated.json `
  --static analysis/derived/6.1.1 `
  --normalized analysis/derived/6.1.1/normalized `
  --catalog-output data/catalog.json `
  --metadata-output data/item-metadata.json `
  --report-output analysis/derived/6.1.1/catalog-report.json `
  --id-map-output analysis/derived/6.1.1/item-id-map.json `
  --id-map-input data/catalog-id-map.json
```

O gerador mantem os IDs atuais, usa `types`, `shape` e `star_positions` do runtime
como fonte da verdade, adiciona itens runtime com `needs_review=true` e marca novas
estrelas sem regra como `rule_status=unknown`. Diferencas substituidas ficam em
`resolved_overrides` no relatorio; nao ha conflitos pendentes nesses campos.

Correspondencias de nomenclatura que nao podem ser inferidas genericamente ficam em
`data/catalog-id-map.json`, por exemplo `rat -> brown_rat` e `knights_sigil ->
knight_s_sigil`.

Para comparar visualmente os conflitos restantes:

```powershell
python analysis/tools/render_conflict_report.py `
  --report analysis/derived/6.1.1/catalog-report.json `
  --curated data/catalog-curated.json `
  --runtime analysis/derived/6.1.1/normalized/items.json `
  --output analysis/derived/6.1.1/conflict-review.md
```

O resultado usa `#` para celulas do item, `*` para estrelas e `@` para sobreposicao.

## Captura atual

Para o APK mod `Backpack-Brawl-v6.1.1-mod-apkvision.apk`, a captura local em
`derived/6.1.1/runtime-final/` registrou 1.196 itens e 488 combinacoes. A projecao
atual preserva 1.196 itens no catalogo, 477 receitas validas, 811 conjuntos de stats
e 1.196 tabelas de nivel. O relatorio em `derived/6.1.1/catalog-report.json` registra
conflitos de shape, tipos e posicoes que ainda exigem revisao.
