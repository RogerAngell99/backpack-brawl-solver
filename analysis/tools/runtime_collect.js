function text(value) {
    if (value === null || value === undefined) return null;
    if (typeof value.content === "string") return value.content;
    try { return value.toString(); } catch (_) { return String(value); }
}

function safe(action, fallback = null) {
    try { return action(); } catch (error) { return fallback; }
}

function method(object, name, argc, args = []) {
    return safe(() => object.method(name, argc).invoke(...args));
}

function field(object, name) {
    return safe(() => object.tryField(name)?.value);
}

function number(value) {
    return typeof value === "number" ? value : safe(() => Number(value), null);
}

function array(value) {
    if (!value) return [];
    let length = safe(() => value.length, null);
    if (typeof length !== "number") length = number(method(value, "get_Count", 0));
    if (typeof length !== "number") length = number(field(value, "_size"));
    if (typeof length !== "number" || length < 0 || length > 10000) return [];
    return Array.from({ length }, (_, index) => {
        const direct = safe(() => value.get(index), null);
        return direct ?? method(value, "get_Item", 1, [index]);
    });
}

function vector(value) {
    if (!value) return null;
    return { x: number(field(value, "x")), y: number(field(value, "y")), z: number(field(value, "z")) };
}

function enumValue(value) {
    if (typeof value === "number") return value;
    return number(field(value, "value__")) ?? text(value);
}

function itemTypeName(value) {
    return text(method(value, "get_itemType", 0)) || text(method(value, "get_name", 0));
}

function point(value) {
    if (!value) return null;
    return { x: number(field(value, "x")), y: number(field(value, "y")) };
}

function itemId(value) {
    if (!value) return null;
    return text(method(value, "get_id", 0));
}

function describeReference(value, depth = 0, seen = new Set()) {
    if (value === null || value === undefined) return null;
    if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") return value;
    if (depth > 2) return { class: safe(() => value.class.fullName, "?") };
    const address = safe(() => value.handle.toString(), null);
    if (address && seen.has(address)) return { class: safe(() => value.class.fullName, "?"), circular: true };
    if (address) seen.add(address);
    const result = { class: safe(() => value.class.fullName, "?") };
    for (const fieldName of ["_id", "_value", "_statType", "_typeName", "_modifierType", "_customDisplayName", "_coinPrice", "_rarity", "_itemLayer", "_unlockSource", "_useDynamicLevels", "_hasShortLocalizedName"]) {
        const current = field(value, fieldName);
        if (current !== null && current !== undefined) {
            const enumName = safe(() => current.type.class.isEnum, false);
            if (enumName) {
                result[fieldName] = { name: text(current), value: enumValue(current) };
            } else {
                result[fieldName] = typeof current === "object" ? describeReference(current, depth + 1, seen) : current;
            }
        }
    }
    return result;
}

function readStats(stats) {
    return array(stats).map(stat => ({
        class: safe(() => stat.class.fullName, "?"),
        value: number(method(stat, "get_value", 0)),
        serialized: describeReference(stat),
    }));
}

function readShape(shape) {
    return array(method(shape, "get_points", 0)).map(point);
}

function readSlots(setup, star = false) {
    const slots = array(method(setup, star ? "get_starSlots" : "get_slots", 0));
    return slots.map(slot => {
        const transform = method(slot, "get_transform", 0);
        return {
            name: text(method(slot, "get_name", 0)),
            position: vector(method(transform, "get_localPosition", 0)),
        };
    });
}

function positions(slots) {
    return slots.map(slot => slot.position).filter(position => position).map(position => [position.x, position.y]);
}

function readLevels(levels) {
    if (!levels) return null;
    const max = number(method(levels, "get_maxLevel", 0));
    const result = { max_level: max, levels: [] };
    if (max === null || max === undefined || max > 64) return result;
    for (let level = 0; level <= max; level++) {
        const value = method(levels, "GetLevel", 1, [level]);
        result.levels.push({ level, value: describeReference(value) });
    }
    return result;
}

function readItem(builder, id) {
    const prefab = method(builder, "get_itemPrefabSetup", 0);
    const levels = method(builder, "get_levels", 0);
    const types = array(method(builder, "get_itemTypes", 0)).map(itemTypeName);
    const slots = readSlots(prefab);
    const starPositions = readSlots(prefab, true);
    return {
        id,
        display_name: text(method(builder, "get_displayName", 0)),
        asset_id: text(method(prefab, "get_assetId", 0)),
        item_types: types,
        rarity: enumValue(method(builder, "get_rarity", 0)),
        layer: enumValue(method(builder, "get_itemLayer", 0)),
        base_shape: positions(slots),
        star_shape: positions(starPositions),
        slots,
        star_positions: starPositions,
        stats: readStats(method(builder, "get_defaultStats", 0)),
        levels: readLevels(levels),
        star_condition: describeReference(field(builder, "_starCondition")),
        on_init: describeReference(method(builder, "get_onInit", 0)),
        on_update: describeReference(method(builder, "get_onUpdate", 0)),
    };
}

function readRecipes(resources, image) {
    const output = [];
    for (const className of ["Model.ItemCombinations.RecipeData", "Model.ItemCombinations.ItemCombinationData"]) {
        const klass = safe(() => image.class(className), null);
        if (!klass) continue;
        const objects = safe(() => resources.method("FindObjectsOfTypeAll", 1).invoke(klass.type.object), null);
        for (const asset of array(objects)) {
            const recipes = className.endsWith("RecipeData") ? array(field(asset, "combinations")) : array(field(asset, "recipes"));
            for (const recipe of recipes) {
                const primary = field(recipe, "primary");
                const result = field(recipe, "result");
                const secondaries = array(field(recipe, "secondaries"));
                output.push({
                    primary: itemId(primary),
                    secondaries: secondaries.map(itemId),
                    result: itemId(result),
                    source_class: className,
                });
            }
        }
    }
    return output;
}

let nextChunk = 0;

function collect() {
    send({ type: "runtime_stage", stage: "assemblies" });
    const assembly = Il2Cpp.domain.assembly("Assembly-CSharp");
    const core = Il2Cpp.domain.assembly("UnityEngine.CoreModule");
    send({ type: "runtime_stage", stage: "classes" });
    const image = assembly.image;
    const resourcesClass = core.image.class("UnityEngine.Resources");
    const itemClass = image.class("ItemDefinitions.ConfigurableItemDefinition");
    send({ type: "runtime_stage", stage: "find_objects", class_name: itemClass.fullName });
    const all = resourcesClass.method("FindObjectsOfTypeAll", 1).invoke(itemClass.type.object);
    send({ type: "runtime_stage", stage: "objects_found", count: all.length });
    const builders = array(all);
    const start = nextChunk;
    const end = Math.min(start + 100, builders.length);
    nextChunk = end;
    const items = [];
    const seen = new Set();
    for (const builder of builders.slice(start, end)) {
        const id = text(method(builder, "get_id", 0));
        if (!id || seen.has(id)) continue;
        seen.add(id);
        items.push(readItem(builder, id));
    }
    return {
        items,
        recipes: end >= builders.length ? readRecipes(resourcesClass, image) : [],
        item_count: builders.length,
        chunk: { start, end, total: builders.length, complete: end >= builders.length },
        collected_at: new Date().toISOString(),
    };
}

function attempt() {
    try {
        Il2Cpp.perform(() => {
            try {
                const data = collect();
                send({ type: "runtime_chunk", data });
            } catch (error) {
                send({ type: "runtime_error", error: String(error) });
            }
        });
    } catch (error) {
        send({ type: "runtime_error", error: String(error) });
    }
}

attempt();
setTimeout(attempt, 8000);
setTimeout(attempt, 18000);
setTimeout(attempt, 30000);
setTimeout(attempt, 45000);
setTimeout(attempt, 60000);
setTimeout(attempt, 75000);
setTimeout(attempt, 90000);
setTimeout(attempt, 105000);
setTimeout(attempt, 120000);
setTimeout(attempt, 135000);
setTimeout(attempt, 150000);
setTimeout(attempt, 165000);
setTimeout(attempt, 180000);
setTimeout(attempt, 195000);
setTimeout(attempt, 210000);
setTimeout(attempt, 225000);
setTimeout(attempt, 240000);
