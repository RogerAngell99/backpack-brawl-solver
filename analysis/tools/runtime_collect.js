function text(value) {
    if (value === null || value === undefined) return null;
    if (typeof value.content === "string") return value.content;
    try { return value.toString(); } catch (_) { return String(value); }
}

const PROFILE = globalThis.RUNTIME_CAPTURE_PROFILE || "full";

function absent(value) {
    if (value === null || value === undefined) return true;
    return safe(() => value.isNull(), false) || safe(() => value.handle.isNull(), false);
}

function safe(action, fallback = null) {
    try { return action(); } catch (error) { return fallback; }
}

function method(object, name, argc, args = []) {
    return safe(() => object.method(name, argc).invoke(...args));
}

function field(object, name) {
    return safe(() => {
        const value = object.tryField(name)?.value;
        return absent(value) ? null : value;
    });
}

function number(value) {
    return typeof value === "number" ? value : safe(() => Number(value), null);
}

function array(value) {
    if (absent(value)) return [];
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
    if (absent(value)) return null;
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
    if (absent(value)) return null;
    return { x: number(field(value, "x")), y: number(field(value, "y")) };
}

function itemId(value) {
    if (!value) return null;
    return text(method(value, "get_id", 0));
}

function describeReference(value, depth = 0, seen = new Set()) {
    if (absent(value)) return null;
    if (typeof value === "string" || typeof value === "number" || typeof value === "boolean") return value;
    const content = safe(() => value.content, null);
    if (typeof content === "string") return content;
    if (depth > 2) return { class: safe(() => value.class.fullName, "?") };
    const address = safe(() => value.handle.toString(), null);
    if (address && seen.has(address)) return { class: safe(() => value.class.fullName, "?"), circular: true };
    if (address) seen.add(address);
    const className = safe(() => value.class.fullName, "?");
    const result = { class: className };
    const enumType = safe(() => value.type.class.isEnum, false);
    if (enumType) result.enum_value = enumValue(value);
    for (const fieldName of ["_id", "_value", "_statType", "_typeName", "_modifierType", "_customDisplayName", "_coinPrice", "_rarity", "_itemLayer", "_unlockSource", "_useDynamicLevels", "_hasShortLocalizedName", "_any", "_itemType", "_stat", "_playerStat", "_definition"]) {
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
    if (className.endsWith("ItemStatSelector")) {
        result.type_name = text(field(value, "_typeName"));
        result.item_stat_type = text(method(value, "get_itemStatType", 0));
    }
    return result;
}

function itemReference(value) {
    if (absent(value)) return null;
    return {
        class: safe(() => value.class.fullName, "?"),
        id: text(method(value, "get_id", 0)),
        name: text(method(value, "get_displayName", 0)),
    };
}

function heroReference(value) {
    if (absent(value)) return null;
    const id = text(method(value, "get_id", 0));
    const englishName = text(method(value, "get_englishName", 0));
    const name = text(method(value, "get_name", 0)) || englishName;
    if (!id && !name) return null;
    return {
        id,
        name,
        english_name: englishName,
        npc: field(value, "NPC") === true,
    };
}

function readHeroList(value) {
    if (absent(value)) return null;
    return array(value).map(heroReference).filter(hero => hero);
}

function readHeroes(resourcesClass, image) {
    if (globalThis.RUNTIME_CAPTURE_HEROES) return globalThis.RUNTIME_CAPTURE_HEROES;
    const heroes = new Map();
    const collectionClass = safe(() => image.class("Heroes.HeroCollection"), null);
    if (collectionClass) {
        const objects = safe(() => resourcesClass.method("FindObjectsOfTypeAll", 1).invoke(collectionClass.type.object), []);
        for (const collectionObject of array(objects)) {
            for (const hero of readHeroList(field(collectionObject, "heroes")) || []) {
                if (hero.id) heroes.set(hero.id, hero);
            }
        }
    }
    if (heroes.size === 0) {
        const heroClass = safe(() => image.class("Heroes.Hero"), null);
        if (heroClass) {
            const objects = safe(() => resourcesClass.method("FindObjectsOfTypeAll", 1).invoke(heroClass.type.object), []);
            for (const heroObject of array(objects)) {
                const hero = heroReference(heroObject);
                if (hero?.id) heroes.set(hero.id, hero);
            }
        }
    }
    globalThis.RUNTIME_CAPTURE_HEROES = [...heroes.values()].sort((left, right) => String(left.id).localeCompare(String(right.id)));
    return globalThis.RUNTIME_CAPTURE_HEROES;
}

function readStarCondition(value, depth = 0, seen = new Set()) {
    if (absent(value)) return null;
    if (depth > 8) return { class: safe(() => value.class.fullName, "?"), truncated: true };
    const address = safe(() => value.handle.toString(), null);
    if (address && seen.has(address)) return { class: safe(() => value.class.fullName, "?"), circular: true };
    if (address) seen.add(address);
    const className = safe(() => value.class.fullName, "?");
    const result = { class: className };
    if (className.endsWith("CompoundStarCondition")) {
        result.any = field(value, "_any");
        result.conditions = array(field(value, "_conditions")).map(condition => readStarCondition(condition, depth + 1, seen));
    } else if (className.endsWith("OtherItemIsOfType")) {
        const itemType = field(value, "_itemType");
        result.item_type = text(method(itemType, "get_itemType", 0)) || text(itemType);
    } else if (className.endsWith("OtherItemHasStatOfType")) {
        const stat = field(value, "_stat");
        result.stat = describeReference(stat);
        result.stat_type = text(field(stat, "_typeName"));
    } else if (className.endsWith("OtherItemCanAddSpecificStat")) {
        result.player_stat = describeReference(field(value, "_playerStat"));
    } else if (className.endsWith("OtherItemIsExactly")) {
        result.definition = itemReference(field(value, "_definition"));
    } else if (className.endsWith("CompareCondition")) {
        result.source_a = describeReference(field(value, "_sourceA"));
        result.source_b = describeReference(field(value, "_sourceB"));
        result.mode = enumValue(field(value, "_mode"));
    } else if (className.endsWith("ItemStatCondition")) {
        result.stat = describeReference(field(value, "_itemStatSelector"));
        result.mode = enumValue(field(value, "_mode"));
        result.stat_value = field(value, "_statValue");
    } else {
        for (const fieldName of ["_starCondition", "_condition", "_stat", "_playerStat", "_itemType", "_definition", "_typeName", "_sourceA", "_sourceB", "_mode", "_itemStatSelector", "_statValue"]) {
            const current = field(value, fieldName);
            if (current !== null && current !== undefined) result[fieldName] = describeReference(current);
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
    const includeStats = PROFILE === "items" || PROFILE === "full";
    const includeStars = PROFILE === "stars" || PROFILE === "full";
    const includeDeepActions = PROFILE === "full";
    const connectedHeroes = method(builder, "get_connectedHeroes", 0);
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
        connected_heroes: readHeroList(connectedHeroes),
        connected_heroes_present: !absent(connectedHeroes),
        stats: includeStats ? readStats(method(builder, "get_defaultStats", 0)) : [],
        levels: includeStats ? readLevels(levels) : null,
        star_condition: includeStars ? describeReference(field(builder, "_starCondition")) : null,
        star_condition_graph: includeStars ? readStarCondition(field(builder, "_starCondition")) : null,
        on_init: includeDeepActions ? describeReference(method(builder, "get_onInit", 0)) : null,
        on_update: includeDeepActions ? describeReference(method(builder, "get_onUpdate", 0)) : null,
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
                const hero = className.endsWith("RecipeData") ? heroReference(field(asset, "hero")) : null;
                output.push({
                    primary: itemId(primary),
                    secondaries: secondaries.map(itemId),
                    result: itemId(result),
                    hero,
                    hero_id: hero?.id || null,
                    source_class: className,
                });
            }
        }
    }
    return output;
}

let nextChunk = 0;
let completed = false;
let recipesRead = false;
let starStateInstalled = false;
let starStateSampleTimer = null;

function objectKey(value) {
    return absent(value) ? null : safe(() => value.handle.toString(), null);
}

function collection(value, limit = 128) {
    if (absent(value)) return [];
    const direct = array(value);
    const directValues = direct.filter(entry => !absent(entry));
    if (directValues.length > 0) return directValues.slice(0, limit);
    if (direct.length === 0) return [];
    const enumerator = method(value, "GetEnumerator", 0);
    if (absent(enumerator)) return [];
    const result = [];
    for (let index = 0; index < limit && method(enumerator, "MoveNext", 0) === true; index++) {
        result.push(method(enumerator, "get_Current", 0));
    }
    return result;
}

function itemSummary(value) {
    if (absent(value)) return null;
    const definition = method(value, "get_definition", 0);
    const stats = collection(method(value, "get_stats", 0), 64).map(stat => ({
        class: safe(() => stat.class.fullName, "?"),
        value: number(method(stat, "get_value", 0)),
    }));
    return {
        object: objectKey(value),
        class: safe(() => value.class.fullName, "?"),
        guid: text(method(value, "get_guid", 0)),
        id: text(method(definition, "get_id", 0)),
        name: text(method(definition, "get_displayName", 0)),
        position: point(method(value, "get_position", 0)),
        orientation: enumValue(method(value, "get_orientation", 0)),
        level: number(method(value, "get_level", 0)),
        stats,
    };
}

function pointText(value) {
    const pointValue = point(value);
    return pointValue ? `${pointValue.x},${pointValue.y}` : null;
}

function sameObject(left, right) {
    const leftKey = objectKey(left);
    const rightKey = objectKey(right);
    return leftKey !== null && leftKey === rightKey;
}

function visualStarState(slot) {
    const transform = method(slot, "get_transform", 0);
    const localPosition = method(transform, "get_localPosition", 0);
    const renderer = method(slot, "get_sprite", 0);
    const current = method(renderer, "get_sprite", 0);
    const hasEffect = method(slot, "get_hasEffect", 0);
    const noEffect = method(slot, "get_noEffect", 0);
    return {
        position: vector(localPosition),
        sprite: objectKey(current),
        has_effect: sameObject(current, hasEffect),
        no_effect: sameObject(current, noEffect),
        state: sameObject(current, hasEffect) ? "has_effect" : sameObject(current, noEffect) ? "no_effect" : "other",
    };
}

function updaterSummary(updater) {
    const shape = field(updater, "_thisItem");
    const item = method(shape, "get_item", 0);
    return {
        updater: objectKey(updater),
        shape: objectKey(shape),
        item: itemSummary(item),
    };
}

function discoverStarConditionMethods(image) {
    const classNames = [
        "Model.DefinitionIsSame",
        "Model.DefinitionIsDifferent",
        "Model.OtherItemIsExactly",
        "Model.CompoundStarCondition",
        "Model.OtherItemIsOfType",
    ];
    return classNames.map(className => {
        const klass = safe(() => image.tryClass(className), null);
        if (!klass) return { class: className, methods: [] };
        return {
            class: className,
            methods: klass.methods.map(candidate => ({
                name: candidate.name,
                parameters: candidate.parameterCount,
                parameter_types: candidate.parameters.map(parameter => ({
                    name: parameter.name,
                    type: safe(() => parameter.type.class.fullName, null),
                })),
                return_type: safe(() => candidate.returnType.class.fullName, null),
                address: safe(() => candidate.virtualAddress.toString(), null),
            })),
        };
    });
}

function shapeSummary(shape) {
    if (absent(shape)) return null;
    return {
        shape: objectKey(shape),
        class: safe(() => shape.class.fullName, "?"),
        item: itemSummary(method(shape, "get_item", 0)),
    };
}

function enumerateStarUpdaters(resourcesClass, updaterClass) {
    return array(resourcesClass.method("FindObjectsOfTypeAll", 1).invoke(updaterClass.type.object));
}

function managedObjects(resourcesClass, className) {
    const klass = safe(() => Il2Cpp.domain.assembly("Assembly-CSharp").image.class(className), null);
    if (!klass) return [];
    return array(resourcesClass.method("FindObjectsOfTypeAll", 1).invoke(klass.type.object));
}

function managedItemSnapshot(resourcesClass, className) {
    return managedObjects(resourcesClass, className).map(owner => ({
        owner: objectKey(owner),
        owner_class: className,
        items: collection(method(owner, "get_managedItems", 0), 256).map(shapeSummary),
    }));
}

function emitInventorySnapshot(resourcesClass, updaterCount) {
    const inventories = managedItemSnapshot(resourcesClass, "Inventory");
    const storages = managedItemSnapshot(resourcesClass, "Storage");
    const itemShapes = managedObjects(resourcesClass, "ItemShape").map(shapeSummary).filter(shape => shape);
    send({
        type: "inventory_snapshot",
        inventories,
        storages,
        item_shapes: itemShapes,
        updater_count: updaterCount,
        captured_at: new Date().toISOString(),
    });
}

function emitStarObservation(updater, parameters, result) {
    const [item2, otherItem, otherItems, playerItems, position] = parameters;
    const summary = updaterSummary(updater);
    const observation = {
        updater: summary.updater,
        source: summary.item,
        item2: itemSummary(item2),
        other_item: itemSummary(otherItem),
        other_items: collection(otherItems).map(itemSummary),
        player_items: collection(playerItems).map(itemSummary),
        position: point(position),
        result: result === true,
        captured_at: new Date().toISOString(),
    };
    send({ type: "star_observation", observation });
    probeDefinitionConditions(item2, otherItem, otherItems);
}

function interceptStarMethod(target, onResult) {
    const original = target.nativeFunction;
    target.revert();
    const startIndex = +!target.isStatic | +Il2Cpp.unityVersionIsBelow201830;
    const callback = function (...args) {
        const parameters = target.parameters.map((parameter, index) =>
            Il2Cpp.fromFridaValue(args[index + startIndex], parameter.type),
        );
        const returnValue = original(...args);
        try {
            onResult(this, parameters, Il2Cpp.fromFridaValue(returnValue, target.returnType));
        } catch (error) {
            send({ type: "star_state_error", error: String(error) });
        }
        return returnValue;
    };
    Interceptor.replace(target.virtualAddress, new NativeCallback(callback, target.returnType.fridaAlias, target.fridaSignature));
}

function conditionArgumentSummary(value) {
    if (absent(value)) return null;
    const className = safe(() => value.class.fullName, "?");
    if (className === "Model.Item") return itemSummary(value);
    const values = collection(value, 128);
    if (values.length > 0) {
        const items = values.map(itemSummary);
        if (items.every(item => item !== null)) return { class: className, items };
    }
    return { class: className, reference: describeReference(value) };
}

let conditionProbeDepth = 0;
let conditionProbeDebugSent = false;

function emitStarConditionObservation(conditionClass, parameters, result, source = "runtime_call") {
    send({
        type: "star_condition_observation",
        condition: conditionClass,
        parameters: parameters.map(conditionArgumentSummary),
        result: result === true,
        source,
        captured_at: new Date().toISOString(),
    });
}

function conditionGraphForItem(item) {
    const definition = method(item, "get_definition", 0);
    const root = field(definition, "_starCondition") || method(definition, "get_starCondition", 0);
    if (!root && !conditionProbeDebugSent) {
        conditionProbeDebugSent = true;
        send({
            type: "star_condition_probe_debug",
            item_class: safe(() => item.class.fullName, null),
            definition: describeReference(definition),
        });
    }
    return root;
}

function conditionChildren(condition) {
    return array(method(condition, "get_conditions", 0) || field(condition, "_conditions"));
}

function probeDefinitionConditions(item2, otherItem, otherItems) {
    if (absent(otherItem)) return;
    const root = conditionGraphForItem(item2);
    if (absent(root)) return;
    const visit = condition => {
        if (absent(condition)) return;
        const className = safe(() => condition.class.fullName, "?");
        if (
            className.endsWith("DefinitionIsSame") ||
            className.endsWith("DefinitionIsDifferent") ||
            className.endsWith("OtherItemIsExactly")
        ) {
            try {
                conditionProbeDepth++;
                const result = method(condition, "HasEffect", 3, [item2, otherItem, otherItems]);
                emitStarConditionObservation(className, [item2, otherItem, otherItems], result, "direct_probe");
            } catch (error) {
                send({ type: "star_state_error", error: `condition probe: ${String(error)}` });
            } finally {
                conditionProbeDepth--;
            }
            return;
        }
        for (const child of conditionChildren(condition)) visit(child);
    };
    visit(root);
}

function installDefinitionConditionCapture(image) {
    for (const className of ["Model.DefinitionIsSame", "Model.DefinitionIsDifferent", "Model.OtherItemIsExactly"]) {
        const klass = safe(() => image.tryClass(className), null);
        const conditionMethod = safe(() => klass.method("HasEffect", 3), null);
        if (conditionMethod) {
            interceptStarMethod(conditionMethod, (_condition, parameters, result) =>
                conditionProbeDepth === 0 && emitStarConditionObservation(className, parameters, result),
            );
        }
    }
}

function sampleStarState(resourcesClass, updaterClass) {
    const updaters = enumerateStarUpdaters(resourcesClass, updaterClass);
    const snapshots = [];
    for (const updater of updaters) {
        try {
            const shape = field(updater, "_thisItem");
            const item = method(shape, "get_item", 0);
            if (absent(shape) || absent(item)) continue;
            updater.method("OnTooltipOpen", 1).invoke(shape);
            updater.method("EvaluateSpriteState", 1).invoke(shape);
            const starSlots = collection(field(updater, "_starSlots"), 64);
            snapshots.push({
                ...updaterSummary(updater),
                loaded: field(updater, "_loaded"),
                star_slot_count: starSlots.length,
                stars: starSlots.map(visualStarState),
                captured_at: new Date().toISOString(),
            });
            updater.method("OnTooltipClose", 1).invoke(shape);
        } catch (error) {
            send({ type: "star_state_error", error: `sample: ${String(error)}` });
        }
    }
    send({ type: "star_visual_snapshot", updaters: snapshots, captured_at: new Date().toISOString() });
    emitInventorySnapshot(resourcesClass, updaters.length);
    return updaters.length;
}

function installStarStateCapture() {
    if (starStateInstalled) return;
    const assembly = Il2Cpp.domain.assembly("Assembly-CSharp");
    const core = Il2Cpp.domain.assembly("UnityEngine.CoreModule");
    const image = assembly.image;
    const resourcesClass = core.image.class("UnityEngine.Resources");
    const updaterClass = image.class("ItemDefinitions.ItemStarSlotUpdater");
    send({ type: "star_condition_methods", methods: discoverStarConditionMethods(image) });
    installDefinitionConditionCapture(image);
    const conditionMethod = updaterClass.method("StarConditionHasEffect", 5);
    interceptStarMethod(conditionMethod, emitStarObservation);
    starStateInstalled = true;
    send({ type: "star_state_ready", method: "ItemStarSlotUpdater.StarConditionHasEffect" });
    sampleStarState(resourcesClass, updaterClass);
    starStateSampleTimer = setInterval(() => {
        try {
            Il2Cpp.perform(() => sampleStarState(resourcesClass, updaterClass));
        } catch (error) {
            send({ type: "star_state_error", error: `timer: ${String(error)}` });
        }
    }, 5000);
}

function collect() {
    if (PROFILE === "star-state") {
        installStarStateCapture();
        return { profile: PROFILE, items: [], recipes: [], heroes: [], item_count: null, collected_at: new Date().toISOString() };
    }
    send({ type: "runtime_stage", stage: "assemblies" });
    const assembly = Il2Cpp.domain.assembly("Assembly-CSharp");
    const core = Il2Cpp.domain.assembly("UnityEngine.CoreModule");
    send({ type: "runtime_stage", stage: "classes" });
    const image = assembly.image;
    const resourcesClass = core.image.class("UnityEngine.Resources");
    const heroes = readHeroes(resourcesClass, image);
    if (PROFILE === "minimal") {
        send({ type: "runtime_data", data: { profile: PROFILE, items: [], recipes: [], heroes, item_count: null, collected_at: new Date().toISOString() } });
        return;
    }
    const itemClass = image.class("ItemDefinitions.ConfigurableItemDefinition");
    send({ type: "runtime_stage", stage: "find_objects", class_name: itemClass.fullName });
    const all = resourcesClass.method("FindObjectsOfTypeAll", 1).invoke(itemClass.type.object);
    send({ type: "runtime_stage", stage: "objects_found", count: all.length });
    if (PROFILE === "enumerate") {
        send({ type: "runtime_data", data: { profile: PROFILE, items: [], recipes: [], heroes, item_count: all.length, collected_at: new Date().toISOString() } });
        return;
    }
    if (PROFILE === "recipes") {
        if (!recipesRead) recipesRead = true;
            send({ type: "runtime_data", data: { profile: PROFILE, items: [], recipes: readRecipes(resourcesClass, image), heroes, item_count: all.length, collected_at: new Date().toISOString() } });
        return;
    }
    const builders = array(all);
    if (completed) {
        return { items: [], recipes: [], heroes, item_count: builders.length, chunk: { start: nextChunk, end: nextChunk, total: builders.length, complete: true } };
    }
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
        recipes: (PROFILE === "full" || PROFILE === "hero-scope") && end >= builders.length && !recipesRead ? (recipesRead = true, readRecipes(resourcesClass, image)) : [],
        heroes,
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
                if (data) {
                    send({ type: "runtime_chunk", data });
                    if (data?.chunk?.complete) completed = true;
                }
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
