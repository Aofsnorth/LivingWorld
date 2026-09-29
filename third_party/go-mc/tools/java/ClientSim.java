import net.minecraft.SharedConstants;
import net.minecraft.core.HolderSet;
import net.minecraft.core.Registry;
import net.minecraft.core.RegistryAccess;
import net.minecraft.core.registries.BuiltInRegistries;
import net.minecraft.core.registries.Registries;
import net.minecraft.network.FriendlyByteBuf;
import net.minecraft.network.protocol.common.ClientboundUpdateTagsPacket;
import net.minecraft.network.protocol.configuration.ClientboundRegistryDataPacket;
import net.minecraft.client.multiplayer.RegistryDataCollector;
import net.minecraft.resources.Identifier;
import net.minecraft.resources.ResourceKey;
import net.minecraft.server.Bootstrap;
import net.minecraft.server.packs.PackType;
import net.minecraft.server.packs.repository.ServerPacksSource;
import net.minecraft.server.packs.resources.MultiPackResourceManager;
import net.minecraft.tags.TagKey;

import java.nio.file.Files;
import java.nio.file.Path;
import java.util.List;

/**
 * ClientSim — replays LivingWorld's exact configuration-phase bytes through the
 * REAL 26.3 client registry-loading code (RegistryDataCollector +
 * RegistryDataLoader + DataComponentInitializers), the same path
 * ClientConfigurationPacketListenerImpl.runWithResources takes when a vanilla
 * client joins a third-party server. If this passes, the client can join.
 *
 * Inputs (written by TestDumpConfigStream into the given directory):
 *   knownpacks.bin        — the SelectKnownPacks announcement payload
 *   regdata_manifest.txt  — registry-data payload files in send order
 *   regdata_NN.bin        — per-packet payload (post-packet-id bytes)
 *   updatetags.bin        — the UpdateTags payload
 *
 * Usage: ClientSim <dumpdir>
 */
public class ClientSim {
    public static void main(String[] args) throws Exception {
        if (args.length < 1) {
            System.out.println("usage: ClientSim <dumpdir>");
            System.exit(2);
        }
        Path dir = Path.of(args[0]);
        SharedConstants.tryDetectVersion();
        Bootstrap.bootStrap();

        // 1. Verify the known-packs announcement is exactly the vanilla core
        //    pack a 26.3 client would select (KnownPacksManager matches by
        //    namespace+id+version).
        {
            var buf = wrap(Files.readAllBytes(dir.resolve("knownpacks.bin")));
            int count = buf.readVarInt();
            if (count != 1) throw new IllegalStateException("known pack count " + count);
            String ns = buf.readUtf(), id = buf.readUtf(), ver = buf.readUtf();
            if (!"minecraft".equals(ns) || !"core".equals(id) || !"26.3".equals(ver)) {
                throw new IllegalStateException("announced pack " + ns + "/" + id + "/" + ver);
            }
            System.out.println("knownpacks: " + ns + "/" + id + "/" + ver + " (matches vanilla core)");
        }

        // 2. Feed every registry-data packet into the collector via the real
        //    packet codec.
        var collector = new RegistryDataCollector();
        for (String file : Files.readAllLines(dir.resolve("regdata_manifest.txt"))) {
            if (file.isBlank()) continue;
            var buf = wrap(Files.readAllBytes(dir.resolve(file)));
            ClientboundRegistryDataPacket packet = ClientboundRegistryDataPacket.STREAM_CODEC.decode(buf);
            if (buf.isReadable()) throw new IllegalStateException(file + ": " + buf.readableBytes() + " trailing bytes");
            collector.appendContents(packet.registry(), packet.entries());
        }

        // 3. Feed the UpdateTags payload through its codec.
        {
            var buf = wrap(Files.readAllBytes(dir.resolve("updatetags.bin")));
            ClientboundUpdateTagsPacket packet = ClientboundUpdateTagsPacket.STREAM_CODEC.decode(buf);
            if (buf.isReadable()) throw new IllegalStateException("updatetags: " + buf.readableBytes() + " trailing bytes");
            collector.appendTags(packet.tags());
        }

        // 4. Run the real client load: collectGameRegistries builds the REMOTE
        //    registry layer from the collected entries (data-less entries are
        //    resolved from the resource provider = the vanilla pack, exactly
        //    like KnownPacksManager.createResourceManager does after accepting
        //    minecraft/core/26.3) and then rebuilds all item components, which
        //    is where missing elements crashed the real client before.
        var rm = new MultiPackResourceManager(PackType.SERVER_DATA,
                List.of(ServerPacksSource.createVanillaPackSource().fullResources()));
        var staticAccess = RegistryAccess.fromRegistryOfRegistries(BuiltInRegistries.REGISTRY);
        var access = collector.collectGameRegistries(rm, staticAccess, false);

        // 5. Assertions on the resulting registry access: the elements the
        //    client's item components hard-require must resolve.
        lookupOrThrow(access, "minecraft:block_transformer", "minecraft:shovel");
        lookupOrThrow(access, "minecraft:block_transformer", "minecraft:axe");
        lookupOrThrow(access, "minecraft:block_transformer", "minecraft:hoe");
        lookupOrThrow(access, "minecraft:banner_pattern", "minecraft:flow");
        lookupOrThrow(access, "minecraft:dimension_type", "minecraft:overworld");
        lookupOrThrow(access, "minecraft:timeline", "minecraft:day");
        lookupOrThrow(access, "minecraft:world_clock", "minecraft:overworld");
        lookupOrThrow(access, "minecraft:enchantment", "minecraft:sharpness");
        lookupOrThrow(access, "minecraft:worldgen/biome", "minecraft:plains");
        lookupOrThrow(access, "minecraft:worldgen/biome", "minecraft:dappled_forest");
        lookupOrThrow(access, "minecraft:worldgen/biome", "minecraft:sulfur_caves");

        // Biome raw ids must be the vanilla order (chunk palettes depend on it).
        {
            var biomeReg = access.lookupOrThrow(Registries.BIOME);
            checkId(biomeReg, "minecraft:badlands", 0);
            checkId(biomeReg, "minecraft:dappled_forest", 8);
            checkId(biomeReg, "minecraft:plains", 41);
        }

        // The overworld dimension_type's timeline tag must resolve with the
        // 4 vanilla timelines.
        {
            var timelineReg = access.lookupOrThrow(Registries.TIMELINE);
            var tag = timelineReg.get(TagKey.create(Registries.TIMELINE, Identifier.parse("minecraft:in_overworld")));
            if (tag.isEmpty() || tag.get().size() != 4) {
                throw new IllegalStateException("timeline:in_overworld tag missing or wrong size");
            }
            System.out.println("timeline:in_overworld resolves with " + tag.get().size() + " timelines");
        }

        System.out.println("CLIENT SIM OK: configuration stream loads in the real 26.3 client code");
    }

    private static FriendlyByteBuf wrap(byte[] bytes) {
        return new FriendlyByteBuf(io.netty.buffer.Unpooled.wrappedBuffer(bytes));
    }

    private static void lookupOrThrow(RegistryAccess.Frozen access, String registry, String element) {
        var key = ResourceKey.create(
                ResourceKey.createRegistryKey(Identifier.parse(registry)),
                Identifier.parse(element));
        var opt = access.lookup(key.registryKey());
        if (opt.isEmpty()) throw new IllegalStateException("registry missing: " + registry);
        var el = opt.get().get(key);
        if (el.isEmpty()) throw new IllegalStateException("element missing: " + registry + "/" + element);
        System.out.println("resolved " + registry + "/" + element);
    }

    private static void checkId(Registry<net.minecraft.world.level.biome.Biome> reg, String name, int want) {
        var key = ResourceKey.create(Registries.BIOME, Identifier.parse(name));
        var holder = reg.get(key).orElseThrow(() -> new IllegalStateException("biome missing " + name));
        int id = reg.getId(holder.value());
        if (id != want) throw new IllegalStateException("biome " + name + " id " + id + ", want " + want);
        System.out.println("biome " + name + " raw id " + id);
    }
}
