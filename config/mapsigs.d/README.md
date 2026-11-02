# mapsigs.d

Flash signature rows. Numbered `*.yaml` files are read in filename order and
concatenated; the first row that stores a name wins. Each shard is a full YAML
document with a `mapsigs:` list. Do not reorder rows inside a shard, and do not
cut inside a pattern list or `also` group.

```
# Byte patterns that locate a calibration map, curve, or scalar in flash.
# Rows run from top to bottom. The first row that stores a name wins.
# {name:MID} splices a RAM address from signatures.yaml. {slot:N} splices
# a call-table word from that file.
#
# pattern   A window that occurs once. XX is the address word inside it.
#           A list is that many rows, one per pattern, in order.
#           {name:MID} and {slot:N} splice the same way as a signature.
# at        Bytes from the hit to that word. Omitted means 2.
#           Negative reads the pointer in front of the hit.
# open      Find this copy first. The DB00 window around it limits pattern.
#           The copy has to occur once unless first is set.
# first     With open, keep the first copy when several exist.
# from      Lowest address to search. An omitted open starts at 0x820000.
# single    With open, keep the row only when pattern occurs once in the window.
# absent    Skip this row when this pattern occurs in the image.
# frame     The pointer at `at` is the 6-byte segmented form.
# far       The pointer at `at` is the 4-byte segmented form.
# deref     Read this many further words at the decoded address.
# derefat   Bytes added before the first of those reads.
# dereffar  That read is the 4-byte segmented pointer.
# add       Bytes from the decoded pointer to the map body.
#           With anchor, bytes from a map this list already found.
# anchor    That earlier map. Its axis words are used unless this row sets its own.
# rows      Row-axis point count, when the axes sit in front of the body.
# cols      Column-axis point count. A curve sets cols and leaves rows out.
# plain     Keep that count when no axis is stored in front of the body.
# xbits     Column axis-point width, 8 or 16. Omitted means 8 when cols is set.
# ybits     Row axis-point width, 8 or 16. Omitted means 8 when rows is set.
# xat, yat  Bytes from the hit to the F2 that loads that axis header from RAM.
#           The count byte at the header has to match cols or rows.
# yord      The lookup indexes the rows directly: an ordinal row axis of rows points.
# bits      Cell width the lookup reads, 8 or 16.
# table     The hit is the count byte of an axis-point table.
# xtable    Column table, by name. Its count has to match cols.
# ytable    Row table, by name. Its count has to match rows.
# also      More maps from the same hit. Each copies this row with its own
#           name and add, and runs right after it. The pointer is not read
#           as a packed header in front of a copy; set cols for its axes.
#
# A later row with the same map name fills it only when the earlier window missed.
```
