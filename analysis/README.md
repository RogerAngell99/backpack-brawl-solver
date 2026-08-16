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

Perfis de diagnostico disponiveis: `minimal`, `enumerate`, `items`, `stars`,
`recipes`, `hero-scope`, `star-state` e `full`. O modo pode ser fixado com `--mode attach` ou `--mode spawn`;
`auto` tenta spawn e cai para attach quando o Android rejeita spawn.

O coletor usa o app ja instalado no aparelho via ADB/Frida, nao instala APK nem envia
dados para fora. Se a carga de conteudo ainda nao estiver pronta, ele aguarda e tenta
novamente; o resultado informa a quantidade capturada e os erros por objeto.

Nota operacional: os testes controlados mostraram que o PID permaneceu estavel em
attach e spawn. O problema observado era uma corrida de inicializacao: a enumeracao
retornava 0 e depois 1.196 objetos quando a materializacao terminava. O coletor agora
registra PID, modo, estagios, erros completos e logcat em cada sessao.

Para juntar a captura com assets e localizacao:

```powershell
python analysis/tools/normalize_runtime.py `
  --static analysis/derived/6.1.1 `
  --runtime analysis/derived/6.1.1/runtime-deep-attach-1/runtime.json `
  --scope-runtime analysis/derived/6.1.1/hero-scope-4/runtime.json `
  --output analysis/derived/6.1.1/normalized
```

## Modelo de dados

O coletor preserva os conceitos do cliente: `asset_id`, `base_shape`, `star_shape`,
`stats`, `levels`, `star_condition`, `star_condition_graph` e `recipes`. A conversao
para o catalogo do solver e feita por `build_catalog.py`. Para todos os itens,
`types`, `shape` e `star_positions` do runtime substituem os valores curados. O grafo
runtime e embutido em `data/catalog.json` quando existe; regras curadas continuam como
fallback para itens sem grafo. Estados dependentes de contexto sem informacao suficiente
retornam `unknown` e nao sao contados arbitrariamente.
Posicoes vindas do runtime Unity sao coletadas como `[x, y]` e convertidas para
`[-y, x]` no contrato canonico `[row, col]` durante a normalizacao. A inversao de Y
e necessaria porque Unity cresce para cima, enquanto o solver cresce para baixo. Os
artefatos estaticos ja sao gravados nesse contrato.
Os tipos de stat materializados ficam em `stat_types` no catalogo para avaliar
condicoes como `OtherItemHasStatOfType`; `hero_scope` registra disponibilidade por
heroi e distingue itens `shared`.

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

A captura profunda tambem preserva `star_condition_graph` no metadata. Ela encontrou
528 arvores de condicao, incluindo `OtherItemIsOfType`, `OtherItemHasStatOfType`,
`DefinitionIsSame`, `DefinitionIsDifferent` e condicoes de modificacao. Isso descreve
a regra estrutural; o estado ativo ainda depende do layout e do contexto de combate.

Veja a matriz de classes, itens candidatos e cenarios em
[`docs/star-condition-coverage.md`](docs/star-condition-coverage.md). O procedimento
reprodutivel de captura esta em
[`docs/star-condition-capture-protocol.md`](docs/star-condition-capture-protocol.md).
O escopo por heroi, incluindo itens shared e filtros de inclusao/exclusao, esta em
[`docs/hero-scope.md`](docs/hero-scope.md).

Para capturar o resultado real da avaliacao de estrelas no inventario atual, use o perfil
focado abaixo depois que o jogo estiver carregado:

```powershell
python analysis/tools/collect_runtime.py `
  --mode attach `
  --profile star-state `
  --wait 60 `
  --output analysis/derived/6.1.1/star-state-calibration
```

Esse perfil intercepta `ItemStarSlotUpdater.StarConditionHasEffect`, preserva o retorno
original e registra o contexto runtime, a coordenada e o estado visual de cada estrela.
Para comparacoes de definicao, ele tambem executa probes diretos nos nos
`DefinitionIsSame` e `DefinitionIsDifferent`, evitando que um
`CompoundStarCondition` com `any` esconda o resultado de um ramo.
O relatorio pode ser gerado com:

```powershell
python analysis/tools/summarize_star_state.py `
  --runtime analysis/derived/6.1.1/star-state-calibration/runtime.json `
  --metadata data/item-metadata.json `
  --output analysis/derived/6.1.1/star-state-calibration/report.json
```

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

Para gerar orientacoes-base dos sprites a partir da proporcao do shape e do Sprite:

```powershell
python analysis/tools/build_visual_metadata.py `
  --catalog data/catalog.json `
  --normalized analysis/derived/6.1.1/normalized/items.json `
  --assets analysis/derived/6.1.1/items/assets.json `
  --asset-root . `
  --output data/item-visual-metadata.json
```

Essa inferencia cobre apenas transposicoes de proporcao. Shapes quadrados ou assets
com pivots assimetricos continuam candidatos a revisao visual manual.

## Captura atual

Para o APK mod `Backpack-Brawl-v6.1.1-mod-apkvision.apk`, a captura profunda em
`derived/6.1.1/runtime-deep-attach-1/` registrou 1.196 itens e 488 combinacoes. A
projecao atual preserva 1.196 itens no catalogo, 477 receitas validas, 811 conjuntos
de stats, 1.196 tabelas de nivel, 528 arvores de condicao e escopo para 1.196 itens. O relatorio em
`derived/6.1.1/catalog-report.json` registra os overrides runtime e as lacunas que
continuam dependentes de contexto.
