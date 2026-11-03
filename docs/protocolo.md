# Protocolo VaiJunto 1.0

## Transporte e enquadramento

- transporte: TCP/IP;
- codificação: JSON em UTF-8;
- enquadramento: uma mensagem JSON por linha, terminada por `\n`;
- tamanho máximo: 64 KiB por mensagem;
- timeout de inatividade no servidor: 10 minutos;
- timeout de escrita: 10 segundos;
- duração da sessão: uma conexão TCP.

TCP é um fluxo de bytes, não uma sequência de mensagens. O delimitador de linha
permite reconstruir mensagens mesmo quando uma leitura recebe dados parciais ou
mais de uma mensagem. O servidor rejeita JSON inválido, campos desconhecidos,
versão incompatível e mensagens acima do limite.

## Envelope comum

Requisição:

```json
{
  "versao": "1.0",
  "id": "req-1",
  "operacao": "buscar_itinerarios",
  "dados": {}
}
```

Resposta:

```json
{
  "versao": "1.0",
  "id": "req-1",
  "sucesso": true,
  "codigo": "ok",
  "mensagem": "itinerários encontrados",
  "dados": []
}
```

`id` correlaciona resposta e requisição. `dados` muda conforme a operação.

## Autenticação e sessão

### `registrar_usuario`

```json
{
  "usuario_id": "gabriel",
  "senha": "senha123",
  "perfil": "passageiro"
}
```

Os perfis permitidos são `motorista` e `passageiro`. O servidor gera um salt
aleatório e armazena somente SHA-256 de `salt + senha`. O cadastro fica em
memória.

### `iniciar_sessao`

```json
{
  "usuario_id": "gabriel",
  "senha": "senha123"
}
```

Após autenticar, a conexão fica vinculada ao ID e perfil. Operações de negócio
antes do login retornam `sessao_obrigatoria`; operações do perfil errado
retornam `perfil_nao_autorizado`.

O protocolo acadêmico não usa TLS. Portanto, a senha fica protegida no estado do
servidor, mas não durante o transporte. Em produção seria obrigatório adicionar
TLS.

## Operações do motorista

### `criar_carona`

```json
{
  "id": "carona-1",
  "horario_saida": "2026-09-17T08:00:00-03:00",
  "rota": ["Feira", "Alagoinhas", "Salvador"],
  "capacidade": 4,
  "precos_centavos": [1000, 1500],
  "duracoes_minutos": [90, 120]
}
```

Uma rota com três cidades produz dois trechos. Preços são inteiros em centavos.
Cada duração estimada é positiva e pertence ao trecho da mesma posição. O
servidor calcula saída e chegada de cada trecho: a chegada de um trecho é a
saída do seguinte trecho da mesma carona. O motorista é obtido da sessão, não
do JSON enviado pelo cliente.

### `listar_caronas_motorista`

Dados da requisição: `{}`. A resposta informa cada trecho, capacidade,
disponibilidade e passageiros confirmados com sua quantidade de assentos.

### `cancelar_carona`

```json
{"id_carona": "carona-1"}
```

Somente o motorista que publicou a carona pode cancelá-la. A carona deixa de
aparecer nas buscas. Se houver reservas confirmadas que a utilizem, cada uma é
cancelada por inteiro e todos os seus trechos — inclusive trechos de outras
caronas no mesmo itinerário — recebem os assentos de volta. Isso impede uma
reserva parcialmente confirmada.

## Operações do passageiro

### `buscar_itinerarios`

```json
{
  "origem": "FEIRA",
  "destino": "Salvador",
  "quantidade_assentos": 2,
  "data_desejada": "2026-09-17T00:00:00-03:00"
}
```

As cidades são comparadas sem diferenciar maiúsculas e minúsculas. Cada opção
contém os trechos, `carona_id`, `ordem`, saída, chegada, preço e disponibilidade. Só são
retornados caminhos com vagas suficientes em todos os trechos. Os resultados
são ordenados por preço total crescente. A busca aceita até oito trechos por
itinerário, examina no máximo mil candidatos e devolve no máximo 20 resultados.

A primeira carona deve sair em `data_desejada` ou depois. Ao trocar de carona,
a próxima saída precisa ocorrer pelo menos 30 minutos após a chegada anterior.
Assim, o passageiro pode aguardar horas ou dias em uma cidade intermediária,
mas não recebe uma conexão que já partiu.

### `confirmar_reserva`

```json
{
  "id_reserva": "reserva-a1b2",
  "quantidade_assentos": 2,
  "trechos": [
    {"carona_id": "carona-1", "ordem": 0},
    {"carona_id": "carona-1", "ordem": 1}
  ]
}
```

O servidor ignora a disponibilidade observada na busca e revalida tudo dentro
do mutex. Se um trecho falhar, nenhum assento é alterado.

### `consultar_reservas`

Dados da requisição: `{}`. Retorna somente reservas do passageiro autenticado,
incluindo status `confirmada` ou `cancelada`.

### `cancelar_reserva`

```json
{"id_reserva": "reserva-a1b2"}
```

Somente o dono pode cancelar. Os assentos são devolvidos a todos os trechos e o
status muda para `cancelada`. Uma segunda tentativa é rejeitada.

## Erros principais

- `mensagem_invalida`: JSON malformado ou envelope inválido;
- `versao_incompativel`: versão diferente de `1.0`;
- `id_obrigatorio`: correlação ausente;
- `dados_invalidos`: corpo ausente, malformado ou com campo desconhecido;
- `autenticacao_falhou`: usuário ou senha inválidos;
- `sessao_obrigatoria`: operação antes do login;
- `perfil_nao_autorizado`: perfil errado para a operação;
- `carona_nao_criada`, `carona_nao_cancelada`, `busca_invalida`,
  `reserva_nao_confirmada` e `reserva_nao_cancelada`: falhas das regras de negócio;
- `operacao_desconhecida`: operação não implementada.

## Desconexões e falhas

Uma requisição não cria reserva provisória. A confirmação acontece inteira sob
o mutex antes da resposta. Portanto, se o cliente cair antes de enviar a
mensagem, nada é bloqueado; se cair depois da confirmação, a reserva permanece
confirmada e pode ser consultada/cancelada em uma nova sessão. JSON parcial ou
inválido afeta somente a conexão que o enviou.
